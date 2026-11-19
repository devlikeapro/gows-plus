package sqlstorage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikeapro/gows/storage"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

func newCallLogTestStore(t *testing.T) *SqlCallLogStore {
	t.Helper()
	gc, err := New("sqlite3", "file:"+filepath.Join(t.TempDir(), "gows.db")+"?_foreign_keys=on", waLog.Noop)
	require.NoError(t, err)
	t.Cleanup(func() { _ = gc.Close() })
	return gc.NewCallLogStorage()
}

func callLog(id string, chat types.JID, at time.Time, result waSyncAction.CallLogRecord_CallResult, duration int64) *storage.CallLog {
	return &storage.CallLog{
		ID:        id,
		Chat:      chat,
		Timestamp: at,
		FromMe:    true,
		Record: &waSyncAction.CallLogRecord{
			CallID:     proto.String(id),
			CallResult: result.Enum(),
			Duration:   proto.Int64(duration),
			StartTime:  proto.Int64(at.UnixMilli()),
			IsIncoming: proto.Bool(false),
			IsVideo:    proto.Bool(false),
		},
	}
}

func callIDs(logs []*storage.CallLog) []string {
	out := make([]string, 0, len(logs))
	for _, l := range logs {
		out = append(out, l.ID)
	}
	return out
}

func TestCallLogsByChatNewestFirst(t *testing.T) {
	store := newCallLogTestStore(t)
	alice := types.NewJID("972501234567", types.DefaultUserServer)
	bob := types.NewJID("200000000000002", types.HiddenUserServer)
	now := time.Unix(1_794_815_000, 0)

	require.NoError(t, store.UpsertCallLogs([]*storage.CallLog{
		callLog("CALL-1", alice, now, waSyncAction.CallLogRecord_CONNECTED, 60),
		callLog("CALL-2", bob, now.Add(time.Minute), waSyncAction.CallLogRecord_MISSED, 0),
		callLog("CALL-3", alice, now.Add(2*time.Minute), waSyncAction.CallLogRecord_REJECTED, 0),
	}))

	got, err := store.GetCallLogs(storage.CallLogFilter{Jid: &alice}, storage.Pagination{})
	require.NoError(t, err)
	require.Equal(t, []string{"CALL-3", "CALL-1"}, callIDs(got))

	all, err := store.GetCallLogs(storage.CallLogFilter{}, storage.Pagination{})
	require.NoError(t, err)
	require.Equal(t, []string{"CALL-3", "CALL-2", "CALL-1"}, callIDs(all))

	page, err := store.GetCallLogs(storage.CallLogFilter{}, storage.Pagination{Limit: 1, Offset: 1})
	require.NoError(t, err)
	require.Equal(t, []string{"CALL-2"}, callIDs(page))

	// the record survives the round trip: result, duration and the chat
	require.Equal(t, waSyncAction.CallLogRecord_CONNECTED, got[1].Record.GetCallResult())
	require.Equal(t, int64(60), got[1].Record.GetDuration())
	require.Equal(t, alice, got[1].Chat)
	require.True(t, got[1].FromMe)

	none, err := store.GetCallLogs(storage.CallLogFilter{Jid: &types.JID{User: "000", Server: types.DefaultUserServer}}, storage.Pagination{})
	require.NoError(t, err)
	require.NotNil(t, none)
	require.Empty(t, none)
}

// The same call arrives from history sync and from the app-state collection: one row, the latest copy wins.
func TestCallLogUpsertReplacesByCallID(t *testing.T) {
	store := newCallLogTestStore(t)
	alice := types.NewJID("972501234567", types.DefaultUserServer)
	now := time.Unix(1_794_815_000, 0)

	require.NoError(t, store.UpsertCallLogs([]*storage.CallLog{callLog("CALL-1", alice, now, waSyncAction.CallLogRecord_ONGOING, 0)}))
	require.NoError(t, store.UpsertCallLogs([]*storage.CallLog{callLog("CALL-1", alice, now, waSyncAction.CallLogRecord_CONNECTED, 95)}))

	got, err := store.GetCallLogs(storage.CallLogFilter{}, storage.Pagination{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, waSyncAction.CallLogRecord_CONNECTED, got[0].Record.GetCallResult())
	require.Equal(t, int64(95), got[0].Record.GetDuration())
}

// A batch may hold the same call twice (and entries without a call id): one statement must not touch a row twice.
func TestCallLogUpsertBatchWithDuplicates(t *testing.T) {
	store := newCallLogTestStore(t)
	alice := types.NewJID("972501234567", types.DefaultUserServer)
	now := time.Unix(1_794_815_000, 0)

	require.NoError(t, store.UpsertCallLogs([]*storage.CallLog{
		callLog("CALL-1", alice, now, waSyncAction.CallLogRecord_MISSED, 0),
		nil,
		{ID: ""},
		callLog("CALL-1", alice, now, waSyncAction.CallLogRecord_CONNECTED, 12),
	}))

	got, err := store.GetCallLogs(storage.CallLogFilter{}, storage.Pagination{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, int64(12), got[0].Record.GetDuration())
}

// History sync can carry the whole call log in one batch: more rows than one statement may bind.
func TestCallLogUpsertLargeBatch(t *testing.T) {
	store := newCallLogTestStore(t)
	alice := types.NewJID("972501234567", types.DefaultUserServer)
	now := time.Unix(1_794_815_000, 0)

	logs := make([]*storage.CallLog, 0, 2*callLogUpsertChunk+7)
	for i := 0; i < cap(logs); i++ {
		logs = append(logs, callLog(time.Duration(i).String()+"-call", alice, now.Add(time.Duration(i)*time.Second), waSyncAction.CallLogRecord_CONNECTED, int64(i)))
	}
	require.NoError(t, store.UpsertCallLogs(logs))

	got, err := store.GetCallLogs(storage.CallLogFilter{Jid: &alice}, storage.Pagination{})
	require.NoError(t, err)
	require.Len(t, got, len(logs))
}
