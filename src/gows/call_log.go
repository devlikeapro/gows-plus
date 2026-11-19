package gows

import (
	"time"

	"github.com/devlikeapro/gows/storage"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// CallLogEvent is emitted for every call log entry synced through the app-state call_log collection:
// a call that has just ended on any of the account's devices, or the whole call log on a full app-state resync.
//
// It is the only place a linked device learns about a 1:1 call placed from the phone - the call signalling
// (events.CallOffer and friends) reaches it only for incoming and group calls.
type CallLogEvent struct {
	*storage.CallLog
}

// callLogStartTime reads CallLogRecord.startTime, which is in milliseconds; a value in seconds is accepted too
func callLogStartTime(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	if value < 1e12 {
		return time.Unix(value, 0)
	}
	return time.UnixMilli(value)
}

// callLogChat finds the chat a call belongs to: the group of a group call, otherwise the first party that is
// not this account - the caller of an incoming call, the callee of an outgoing one.
// The app-state index ([call_log, ...]) is the last resort.
func callLogChat(record *waSyncAction.CallLogRecord, index []string, own func(types.JID) bool) types.JID {
	if record.GetGroupJID() != "" {
		group, err := types.ParseJID(record.GetGroupJID())
		if err == nil && group.Server == types.GroupServer {
			return group
		}
	}
	candidates := []string{record.GetCallCreatorJID()}
	for _, participant := range record.GetParticipants() {
		candidates = append(candidates, participant.GetUserJID())
	}
	if len(index) > 1 {
		candidates = append(candidates, index[1:]...)
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		jid, err := types.ParseJID(candidate)
		if err != nil || jid.User == "" {
			continue
		}
		if jid.Server != types.DefaultUserServer && jid.Server != types.HiddenUserServer && jid.Server != types.GroupServer {
			continue
		}
		jid = jid.ToNonAD()
		if own != nil && own(jid) {
			continue
		}
		return jid
	}
	return types.EmptyJID
}

// callLogFromRecord converts a WhatsApp call log record; nil when the record has no call id
func callLogFromRecord(record *waSyncAction.CallLogRecord, index []string, own func(types.JID) bool) *storage.CallLog {
	if record == nil || record.GetCallID() == "" {
		return nil
	}
	return &storage.CallLog{
		ID:        record.GetCallID(),
		Chat:      callLogChat(record, index, own),
		Timestamp: callLogStartTime(record.GetStartTime()),
		FromMe:    !record.GetIsIncoming(),
		Record:    record,
	}
}

// isOwnJID reports whether the JID is this account, by phone number or by LID
func (gows *GoWS) isOwnJID(jid types.JID) bool {
	if gows == nil || gows.Store == nil {
		return false
	}
	if id := gows.Store.ID; id != nil && jid.Server == types.DefaultUserServer && jid.User == id.User {
		return true
	}
	lid := gows.Store.GetLID()
	return !lid.IsEmpty() && jid.Server == types.HiddenUserServer && jid.User == lid.User
}

func (st *StorageEventHandler) isOwnJID(jid types.JID) bool {
	return st.gows.isOwnJID(jid)
}

func (st *StorageEventHandler) handleAppState(event *events.AppState) {
	if len(event.Index) == 0 || event.Index[0] != appstate.IndexCallLog || event.SyncActionValue == nil {
		return
	}
	log := callLogFromRecord(event.GetCallLogAction().GetCallLogRecord(), event.Index, st.isOwnJID)
	if log == nil {
		st.log.Warnf("Call log mutation without a call id, index %v", event.Index)
		return
	}
	if st.shouldIgnoreJID(log.Chat) {
		return
	}
	if log.Chat.IsEmpty() {
		st.log.Warnf("Call log %v: no chat found (index %v)", log.ID, event.Index)
	}
	err := st.storage.CallLogs.UpsertCallLogs([]*storage.CallLog{log})
	if err != nil {
		st.log.Errorf("Error storing call log %v: %v", log.ID, err)
	} else {
		st.log.Debugf("Stored call log %v in %v (index %v)", log.ID, log.Chat, event.Index)
	}
	if st.gows != nil {
		st.gows.emitEvent(&CallLogEvent{CallLog: log})
	}
}

func (st *StorageEventHandler) saveCallLogsFromHistory(records []*waSyncAction.CallLogRecord) {
	logs := make([]*storage.CallLog, 0, len(records))
	for _, record := range records {
		log := callLogFromRecord(record, nil, st.isOwnJID)
		if log == nil || st.shouldIgnoreJID(log.Chat) {
			continue
		}
		logs = append(logs, log)
	}
	err := st.storage.CallLogs.UpsertCallLogs(logs)
	if err != nil {
		st.log.Errorf("Error storing %v call logs from history: %v", len(logs), err)
		return
	}
	st.log.Debugf("Saved %v call logs from history", len(logs))
}
