package gows

import (
	"context"
	"log/slog"
	"time"

	"github.com/devlikeapro/gows/callbridge"
	"github.com/devlikeapro/gows/voip/call"
	"github.com/devlikeapro/gows/voip/core"
	"github.com/devlikeapro/gows/voip/media"
	voipwa "github.com/devlikeapro/gows/voip/wa"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// CallLifecycleEvent is emitted to WAHA event stream with a stable webhook contract.
type CallLifecycleEvent struct {
	Event     string `json:"event"`
	ID        string `json:"id"`
	From      string `json:"from"`
	Direction string `json:"direction"`
	Status    string `json:"status"`
	Reason    string `json:"reason,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

type callBridgeState struct {
	bridge      *callbridge.Bridge
	browserOpus media.Codec
}

func (gows *GoWS) initCallManager() {
	if gows.callManager != nil {
		return
	}
	gows.callManager = call.NewCallManager(voipwa.NewSocket(gows.Client), slog.Default())
	gows.wireCallManager()
}

func (gows *GoWS) wireCallManager() {
	cm := gows.callManager
	cm.OnIncoming = func(c *call.CallInfo) {
		gows.emitCallLifecycle("call.received", c, "")
	}
	cm.OnStateChange = func(c *call.CallInfo) {
		if c.IsEnded() {
			return
		}
		eventName := callStatusToEvent(c)
		gows.emitCallLifecycle(eventName, c, "")
	}
	cm.OnEnded = func(c *call.CallInfo) {
		reason := string(c.StateData.EndReason)
		if reason == "" {
			reason = string(core.EndCallReasonUnknown)
		}
		eventName := "call.ended"
		if c.StateData.EndReason == core.EndCallReasonDeclined {
			eventName = "call.rejected"
		}
		gows.emitCallLifecycle(eventName, c, reason)
		gows.closeCallBridge()
	}
	cm.OnPeerAudio = func(pcm16 []float32) {
		gows.callBridgeMu.Lock()
		br := gows.callBridge.bridge
		oc := gows.callBridge.browserOpus
		gows.callBridgeMu.Unlock()
		if br == nil || oc == nil {
			return
		}
		pcm48 := media.Upsample16to48(pcm16)
		opus, err := oc.Encode(pcm48)
		if err != nil || len(opus) == 0 {
			return
		}
		_ = br.WriteOpus(opus, 60*time.Millisecond)
	}
}

func callStatusToEvent(c *call.CallInfo) string {
	switch c.StateData.State {
	case core.CallStateRinging, core.CallStateIncomingRinging:
		return "call.ringing"
	case core.CallStateConnecting:
		return "call.connecting"
	case core.CallStateActive:
		return "call.active"
	default:
		return "call.status"
	}
}

func (gows *GoWS) emitCallLifecycle(eventName string, c *call.CallInfo, reason string) {
	dir := "outbound"
	if c.Direction == core.CallDirectionIncoming {
		dir = "inbound"
	}
	gows.emitEvent(&CallLifecycleEvent{
		Event:     eventName,
		ID:        c.CallID,
		From:      c.PeerJid,
		Direction: dir,
		Status:    string(c.StateData.State),
		Reason:    reason,
		Timestamp: time.Now().UnixMilli(),
	})
}

func wrapCallNode(from types.JID, inner *waBinary.Node) *waBinary.Node {
	content := []waBinary.Node{}
	if inner != nil {
		content = append(content, *inner)
	}
	return &waBinary.Node{
		Tag:     "call",
		Attrs:   waBinary.Attrs{"from": from},
		Content: content,
	}
}

func (gows *GoWS) routeCallEvent(event interface{}) {
	gows.initCallManager()
	ctx := gows.Context
	if ctx == nil {
		ctx = context.Background()
	}
	switch evt := event.(type) {
	case *events.CallOffer:
		gows.callManager.HandleCallOffer(ctx, wrapCallNode(evt.From, evt.Data), evt.From)
	case *events.CallAccept:
		gows.callManager.HandleCallAccept(ctx, wrapCallNode(evt.From, evt.Data), evt.From)
	case *events.CallTransport:
		gows.callManager.HandleCallTransport(ctx, wrapCallNode(evt.From, evt.Data), evt.From)
	case *events.CallTerminate:
		gows.callManager.HandleCallTerminate(wrapCallNode(evt.From, evt.Data))
	case *events.CallReject:
		gows.callManager.HandleCallTerminate(wrapCallNode(evt.From, evt.Data))
	}
}

func (gows *GoWS) StartVoipCall(ctx context.Context, peer types.JID, isVideo bool) (string, error) {
	gows.initCallManager()
	return gows.callManager.StartCall(ctx, peer, isVideo)
}

func (gows *GoWS) AcceptVoipCall(ctx context.Context, callID, ownerID string) error {
	gows.initCallManager()
	if ownerID != "" {
		if !gows.claimCallOwner(callID, ownerID) {
			return &call.CallError{"claimed by another client"}
		}
	}
	return gows.callManager.AcceptCall(ctx, callID)
}

func (gows *GoWS) RejectVoipCall(ctx context.Context, from types.JID, callID string) error {
	gows.initCallManager()
	if cm := gows.callManager.CurrentCall(); cm != nil && cm.CallID == callID {
		return gows.callManager.RejectCall(ctx, callID, core.EndCallReasonDeclined)
	}
	return gows.Client.RejectCall(ctx, from, callID)
}

func (gows *GoWS) EndVoipCall(ctx context.Context, callID string) error {
	gows.initCallManager()
	cm := gows.callManager.CurrentCall()
	if cm == nil || cm.CallID != callID {
		return &call.CallError{"no call with id " + callID}
	}
	err := gows.callManager.EndCall(ctx, core.EndCallReasonUserEnded)
	gows.closeCallBridge()
	return err
}

func (gows *GoWS) ExchangeCallWebRTC(offerSDP string) (string, error) {
	gows.initCallManager()
	bridge, answer, err := callbridge.NewBridge(offerSDP, slog.Default())
	if err != nil {
		return "", err
	}
	browserOpus, ocErr := media.NewOpusCodec(48000, 960)
	if ocErr != nil {
		gows.Log.Warnf("browser Opus codec unavailable — call audio disabled: %v", ocErr)
		browserOpus = nil
	}
	bridge.OnBrowserRTP = func(payload []byte) {
		if browserOpus == nil {
			return
		}
		pcm48, decErr := browserOpus.Decode(payload)
		if decErr != nil {
			return
		}
		gows.callManager.FeedCapturedPCM(media.Downsample48to16(pcm48))
	}
	gows.setCallBridge(bridge, browserOpus)
	return answer, nil
}

func (gows *GoWS) GetCallState() *CallLifecycleEvent {
	gows.initCallManager()
	cm := gows.callManager.CurrentCall()
	if cm == nil {
		return nil
	}
	dir := "outbound"
	if cm.Direction == core.CallDirectionIncoming {
		dir = "inbound"
	}
	return &CallLifecycleEvent{
		Event:     callStatusToEvent(cm),
		ID:        cm.CallID,
		From:      cm.PeerJid,
		Direction: dir,
		Status:    string(cm.StateData.State),
		Timestamp: time.Now().UnixMilli(),
	}
}

func (gows *GoWS) setCallBridge(b *callbridge.Bridge, oc media.Codec) {
	gows.callBridgeMu.Lock()
	old := gows.callBridge
	gows.callBridge = callBridgeState{bridge: b, browserOpus: oc}
	gows.callBridgeMu.Unlock()
	if old.bridge != nil {
		old.bridge.Close()
	}
	if old.browserOpus != nil {
		old.browserOpus.Close()
	}
}

func (gows *GoWS) closeCallBridge() {
	gows.callBridgeMu.Lock()
	old := gows.callBridge
	gows.callBridge = callBridgeState{}
	gows.callBridgeMu.Unlock()
	if old.bridge != nil {
		old.bridge.Close()
	}
	if old.browserOpus != nil {
		old.browserOpus.Close()
	}
}

func (gows *GoWS) claimCallOwner(callID, ownerID string) bool {
	gows.callOwnersMu.Lock()
	defer gows.callOwnersMu.Unlock()
	if existing, ok := gows.callOwners[callID]; ok && existing != ownerID {
		return false
	}
	gows.callOwners[callID] = ownerID
	return true
}

func (gows *GoWS) cleanupCalls() {
	if gows.callManager != nil {
		cm := gows.callManager.CurrentCall()
		if cm != nil && !cm.IsEnded() {
			_ = gows.callManager.EndCall(context.Background(), core.EndCallReasonUserEnded)
		}
	}
	gows.closeCallBridge()
	gows.callOwnersMu.Lock()
	gows.callOwners = make(map[string]string)
	gows.callOwnersMu.Unlock()
}
