package sqlstorage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikeapro/gows/storage"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

const testMessageID = "3AAA2EDC18E1045182AE"

func newTestMessageStore(t *testing.T) *SqlMessageStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gows.db")
	container, err := New("sqlite3", "file:"+path+"?_foreign_keys=on", waLog.Noop)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Close() })
	return container.NewMessageStorage()
}

// storedMessage builds a message for the given chat. An empty body means the
// content was not decrypted yet, which is what isRealMessage() reports false on.
func storedMessage(t *testing.T, body string, isReal bool) *storage.StoredMessage {
	t.Helper()
	chat, err := types.ParseJID("120363407715156190@g.us")
	require.NoError(t, err)
	msg := &events.Message{
		Info: types.MessageInfo{
			ID:            testMessageID,
			Timestamp:     time.Unix(1788211385, 0),
			MessageSource: types.MessageSource{Chat: chat},
		},
		Message: &waE2E.Message{},
	}
	if body != "" {
		msg.Message = &waE2E.Message{Conversation: proto.String(body)}
	}
	status := storage.StatusDeliveryAck
	return &storage.StoredMessage{Message: msg, IsReal: isReal, Status: &status}
}

func listMessages(t *testing.T, store *SqlMessageStore) []*storage.StoredMessage {
	t.Helper()
	msgs, err := store.GetAllMessages(
		storage.MessageFilter{},
		storage.Sort{Field: "timestamp", Order: storage.SortDesc},
		storage.Pagination{Limit: 10, Offset: 0},
		false,
	)
	require.NoError(t, err)
	return msgs
}

// A message can reach storage before its content is decrypted, which stamps
// is_real=false. The re-upsert carrying the decrypted content must clear that
// flag, otherwise GetAllMessages (which filters on the column) hides the
// message forever even though its stored data says IsReal=true.
func TestUpsertRefreshesIsRealWhenContentArrivesLater(t *testing.T) {
	store := newTestMessageStore(t)

	require.NoError(t, store.UpsertOneMessage(storedMessage(t, "", false)))
	require.Empty(t, listMessages(t, store), "undecrypted placeholder must stay hidden")

	require.NoError(t, store.UpsertOneMessage(storedMessage(t, "Ok ok , thank you !", true)))

	msgs := listMessages(t, store)
	require.Len(t, msgs, 1, "message must appear once its content arrives")
	require.Equal(t, testMessageID, msgs[0].Info.ID)
	require.True(t, msgs[0].IsReal)
}

// The same column must also be able to go the other way, so a row that was
// stored as real and is later replaced by a protocol message stops being listed.
func TestUpsertRefreshesIsRealWhenMessageBecomesProtocol(t *testing.T) {
	store := newTestMessageStore(t)

	require.NoError(t, store.UpsertOneMessage(storedMessage(t, "Ok ok , thank you !", true)))
	require.Len(t, listMessages(t, store), 1)

	require.NoError(t, store.UpsertOneMessage(storedMessage(t, "", false)))
	require.Empty(t, listMessages(t, store))
}
