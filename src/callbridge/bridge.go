package callbridge

import (
	"log/slog"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

type Bridge struct {
	pc         *webrtc.PeerConnection
	localTrack *webrtc.TrackLocalStaticSample
	log        *slog.Logger

	OnBrowserRTP func(payload []byte)
}

func NewBridge(offerSDP string, log *slog.Logger) (*Bridge, string, error) {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return nil, "", err
	}

	br := &Bridge{pc: pc, log: log}

	localTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeOpus,
			ClockRate:   48000,
			Channels:    2,
			SDPFmtpLine: "minptime=10;useinbandfec=1",
		},
		"audio", "gows",
	)
	if err != nil {
		pc.Close()
		return nil, "", err
	}
	br.localTrack = localTrack

	pc.OnTrack(func(tr *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		go func() {
			for {
				pkt, _, err := tr.ReadRTP()
				if err != nil {
					return
				}
				if br.OnBrowserRTP != nil && len(pkt.Payload) > 0 {
					br.OnBrowserRTP(pkt.Payload)
				}
			}
		}()
	})

	pc.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) {
		if log != nil {
			log.Debug("browser ice state", "state", s.String())
		}
	})

	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offerSDP}); err != nil {
		pc.Close()
		return nil, "", err
	}
	if _, err := pc.AddTrack(localTrack); err != nil {
		pc.Close()
		return nil, "", err
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		pc.Close()
		return nil, "", err
	}
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		pc.Close()
		return nil, "", err
	}
	<-gatherComplete

	return br, pc.LocalDescription().SDP, nil
}

func (b *Bridge) WriteOpus(payload []byte, dur time.Duration) error {
	if b.localTrack == nil {
		return nil
	}
	return b.localTrack.WriteSample(media.Sample{Data: payload, Duration: dur})
}

func (b *Bridge) Close() {
	if b.pc != nil {
		_ = b.pc.Close()
	}
}
