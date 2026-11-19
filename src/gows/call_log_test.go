package gows

import (
	"sync"
	"testing"
	"time"

	gowsLog "github.com/devlikeapro/gows/log"
	"github.com/devlikeapro/gows/storage"
	"github.com/stretchr/testify/require"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// This account: 15550001111 / 100000000000001@lid
var (
	ownPN  = types.NewJID("15550001111", types.DefaultUserServer)
	ownLID = types.NewJID("100000000000001", types.HiddenUserServer)
)

func isOwn(jid types.JID) bool {
	return jid.User == ownPN.User || jid.User == ownLID.User
}

type spyCallLogStorage struct {
	mu       sync.Mutex
	upserted []*storage.CallLog
}

func (s *spyCallLogStorage) UpsertCallLogs(logs []*storage.CallLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upserted = append(s.upserted, logs...)
	return nil
}

func (s *spyCallLogStorage) GetCallLogs(storage.CallLogFilter, storage.Pagination) ([]*storage.CallLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*storage.CallLog(nil), s.upserted...), nil
}

func (s *spyCallLogStorage) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.upserted)
}

func participants(jids ...string) []*waSyncAction.CallLogRecord_ParticipantInfo {
	out := make([]*waSyncAction.CallLogRecord_ParticipantInfo, 0, len(jids))
	for _, j := range jids {
		out = append(out, &waSyncAction.CallLogRecord_ParticipantInfo{UserJID: proto.String(j)})
	}
	return out
}

func TestCallLogChat(t *testing.T) {
	tests := []struct {
		name   string
		record *waSyncAction.CallLogRecord
		index  []string
		want   types.JID
	}{
		{
			name: "incoming 1:1 - the caller",
			record: &waSyncAction.CallLogRecord{
				IsIncoming:     proto.Bool(true),
				CallCreatorJID: proto.String("200000000000002@lid"),
				Participants:   participants("100000000000001@lid"),
			},
			want: types.NewJID("200000000000002", types.HiddenUserServer),
		},
		{
			name: "outgoing 1:1 from another device of this account - the callee, device suffix dropped",
			record: &waSyncAction.CallLogRecord{
				IsIncoming:     proto.Bool(false),
				CallCreatorJID: proto.String("100000000000001:28@lid"),
				Participants:   participants("15550002222:3@s.whatsapp.net"),
			},
			want: types.NewJID("15550002222", types.DefaultUserServer),
		},
		{
			name: "group call - the group",
			record: &waSyncAction.CallLogRecord{
				GroupJID:       proto.String("120363000000000099@g.us"),
				CallCreatorJID: proto.String("200000000000002@lid"),
				Participants:   participants("300000000000003@lid"),
			},
			want: types.NewJID("120363000000000099", types.GroupServer),
		},
		{
			name: "only this account in the record - the index is the last resort",
			record: &waSyncAction.CallLogRecord{
				CallCreatorJID: proto.String("15550001111@s.whatsapp.net"),
			},
			index: []string{appstate.IndexCallLog, "not-a-jid", "15550002222@s.whatsapp.net"},
			want:  types.NewJID("15550002222", types.DefaultUserServer),
		},
		{
			name:   "nothing usable",
			record: &waSyncAction.CallLogRecord{CallCreatorJID: proto.String("100000000000001@lid")},
			index:  []string{appstate.IndexCallLog, "00BB0000000000000000000000000002"},
			want:   types.EmptyJID,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, callLogChat(tt.record, tt.index, isOwn))
		})
	}
}

func TestCallLogStartTime(t *testing.T) {
	require.Equal(t, time.UnixMilli(1_794_815_072_123), callLogStartTime(1_794_815_072_123))
	require.Equal(t, time.Unix(1_794_815_072, 0), callLogStartTime(1_794_815_072))
	require.True(t, callLogStartTime(0).IsZero())
}

func TestCallLogFromRecord(t *testing.T) {
	require.Nil(t, callLogFromRecord(nil, nil, isOwn))
	require.Nil(t, callLogFromRecord(&waSyncAction.CallLogRecord{}, nil, isOwn))

	record := &waSyncAction.CallLogRecord{
		CallID:         proto.String("00AA0000000000000000000000000001"),
		IsIncoming:     proto.Bool(false),
		StartTime:      proto.Int64(1_794_815_072_000),
		CallCreatorJID: proto.String("100000000000001@lid"),
		Participants:   participants("200000000000002@lid"),
	}
	log := callLogFromRecord(record, nil, isOwn)
	require.Equal(t, "00AA0000000000000000000000000001", log.ID)
	require.True(t, log.FromMe)
	require.Equal(t, types.NewJID("200000000000002", types.HiddenUserServer), log.Chat)
	require.Equal(t, time.Unix(1_794_815_072, 0), log.Timestamp)
	require.Same(t, record, log.Record)
}

func newCallLogTestHandler() (*StorageEventHandler, *spyCallLogStorage) {
	calls := &spyCallLogStorage{}
	st := &StorageEventHandler{
		log:     gowsLog.Stdout("test", "ERROR", false),
		storage: &storage.Storage{CallLogs: calls},
	}
	return st, calls
}

func callLogMutation(index []string, record *waSyncAction.CallLogRecord) *events.AppState {
	return &events.AppState{
		Index:           index,
		SyncActionValue: &waSyncAction.SyncActionValue{CallLogAction: &waSyncAction.CallLogAction{CallLogRecord: record}},
	}
}

func TestHandleAppStateStoresCallLog(t *testing.T) {
	st, calls := newCallLogTestHandler()

	st.processEvent(callLogMutation([]string{appstate.IndexCallLog, "1"}, &waSyncAction.CallLogRecord{
		CallID:         proto.String("CALL-1"),
		IsIncoming:     proto.Bool(true),
		CallResult:     waSyncAction.CallLogRecord_MISSED.Enum(),
		CallCreatorJID: proto.String("200000000000002@lid"),
	}))

	require.Len(t, calls.upserted, 1)
	require.Equal(t, "CALL-1", calls.upserted[0].ID)
	require.False(t, calls.upserted[0].FromMe)
	require.Equal(t, waSyncAction.CallLogRecord_MISSED, calls.upserted[0].Record.GetCallResult())
}

func TestHandleAppStateIgnoresOtherMutations(t *testing.T) {
	st, calls := newCallLogTestHandler()

	st.processEvent(&events.AppState{Index: []string{appstate.IndexMute, "972501234567@s.whatsapp.net"}, SyncActionValue: &waSyncAction.SyncActionValue{}})
	st.processEvent(&events.AppState{Index: []string{appstate.IndexCallLog}})
	st.processEvent(&events.AppState{})
	// a call_log mutation without a call id
	st.processEvent(callLogMutation([]string{appstate.IndexCallLog}, &waSyncAction.CallLogRecord{}))

	require.Empty(t, calls.upserted)
}

func TestHandleAppStateRespectsIgnoredJids(t *testing.T) {
	st, calls := newCallLogTestHandler()
	st.ignoreJids = &IgnoreJidsConfig{Groups: true}

	st.processEvent(callLogMutation([]string{appstate.IndexCallLog}, &waSyncAction.CallLogRecord{
		CallID:   proto.String("GROUP-CALL"),
		GroupJID: proto.String("120363000000000099@g.us"),
	}))

	require.Empty(t, calls.upserted)
}

func TestSaveCallLogsFromHistory(t *testing.T) {
	st, calls := newCallLogTestHandler()

	st.saveCallLogsFromHistory([]*waSyncAction.CallLogRecord{
		{CallID: proto.String("H-1"), IsIncoming: proto.Bool(true), CallCreatorJID: proto.String("200000000000002@lid")},
		{},
		nil,
		{CallID: proto.String("H-2"), IsIncoming: proto.Bool(false), Participants: participants("15550002222@s.whatsapp.net")},
	})

	require.Equal(t, []string{"H-1", "H-2"}, []string{calls.upserted[0].ID, calls.upserted[1].ID})
	require.Len(t, calls.upserted, 2)
}

// The history sync handler hands the call log over; the records themselves are covered above.
func TestHandleHistorySyncWithCallLogsOnly(t *testing.T) {
	st, calls := newCallLogTestHandler()

	st.handleHistorySync(&events.HistorySync{Data: &waHistorySync.HistorySync{
		CallLogRecords: []*waSyncAction.CallLogRecord{{CallID: proto.String("H-1"), CallCreatorJID: proto.String("200000000000002@lid")}},
	}})

	require.Eventually(t, func() bool { return calls.count() == 1 }, time.Second, 10*time.Millisecond)
}
