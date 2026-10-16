package sqlstorage

import (
	"errors"
	"path/filepath"
	"strings"
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

const updateTestID = "3EB0C0FFEE0000000000AA"

func newUpdateTestStore(t *testing.T) *SqlMessageStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gows.db")
	container, err := New("sqlite3", "file:"+path+"?_foreign_keys=on", waLog.Noop)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Close() })
	return container.NewMessageStorage()
}

func updateTestMessage(t *testing.T, text string) *storage.StoredMessage {
	t.Helper()
	chat, err := types.ParseJID("120363000000000003@g.us")
	require.NoError(t, err)
	msg := &waE2E.Message{Conversation: proto.String(text)}
	status := storage.StatusDeliveryAck
	return &storage.StoredMessage{
		Message: &events.Message{
			Info: types.MessageInfo{
				ID:            updateTestID,
				Timestamp:     time.Unix(1790000000, 0),
				MessageSource: types.MessageSource{Chat: chat, IsGroup: true},
			},
			Message:    msg,
			RawMessage: msg,
		},
		IsReal: true,
		Status: &status,
	}
}

func TestUpdateMessageNotStored(t *testing.T) {
	store := newUpdateTestStore(t)
	called := false
	err := store.UpdateMessage(updateTestID, func(*storage.StoredMessage) (bool, error) {
		called = true
		return true, nil
	})
	require.ErrorIs(t, err, storage.ErrNotFound)
	require.False(t, called)
}

func TestUpdateMessageWritesOnlyReportedChanges(t *testing.T) {
	store := newUpdateTestStore(t)
	require.NoError(t, store.UpsertOneMessage(updateTestMessage(t, "old")))

	// No change reported: nothing is written, even if the copy was modified
	require.NoError(t, store.UpdateMessage(updateTestID, func(msg *storage.StoredMessage) (bool, error) {
		msg.Message.Message.Conversation = proto.String("not written")
		return false, nil
	}))
	got, err := store.GetMessage(updateTestID)
	require.NoError(t, err)
	require.Equal(t, "old", got.Message.Message.GetConversation())

	// An error is returned as is and nothing is written
	failure := errors.New("failure")
	err = store.UpdateMessage(updateTestID, func(msg *storage.StoredMessage) (bool, error) {
		msg.Message.Message.Conversation = proto.String("not written")
		return true, failure
	})
	require.ErrorIs(t, err, failure)
	got, err = store.GetMessage(updateTestID)
	require.NoError(t, err)
	require.Equal(t, "old", got.Message.Message.GetConversation())

	// A reported change is written
	require.NoError(t, store.UpdateMessage(updateTestID, func(msg *storage.StoredMessage) (bool, error) {
		msg.Message.Message.Conversation = proto.String("new")
		return true, nil
	}))
	got, err = store.GetMessage(updateTestID)
	require.NoError(t, err)
	require.Equal(t, "new", got.Message.Message.GetConversation())
}

func TestEditedAtIsStoredOnlyOnceEdited(t *testing.T) {
	store := newUpdateTestStore(t)
	require.NoError(t, store.UpsertOneMessage(updateTestMessage(t, "old")))

	var data string
	require.NoError(t, store.db.Get(&data, "SELECT data FROM gows_messages WHERE id = $1", updateTestID))
	require.False(t, strings.Contains(data, "EditedAt"), "a message never edited carries no EditedAt")
	got, err := store.GetMessage(updateTestID)
	require.NoError(t, err)
	require.True(t, got.EditedAt.IsZero())

	editedAt := time.Unix(1790000180, 0)
	require.NoError(t, store.UpdateMessage(updateTestID, func(msg *storage.StoredMessage) (bool, error) {
		msg.EditedAt = editedAt
		return true, nil
	}))
	got, err = store.GetMessage(updateTestID)
	require.NoError(t, err)
	require.True(t, got.EditedAt.Equal(editedAt))
}

// A message deleted while it is being updated must stay deleted.
func TestDeleteMessageDuringUpdateIsNotUndone(t *testing.T) {
	store := newUpdateTestStore(t)
	require.NoError(t, store.UpsertOneMessage(updateTestMessage(t, "old")))

	deleted := make(chan error, 1)
	require.NoError(t, store.UpdateMessage(updateTestID, func(msg *storage.StoredMessage) (bool, error) {
		go func() { deleted <- store.DeleteMessage(updateTestID) }()
		time.Sleep(100 * time.Millisecond)
		msg.Message.Message.Conversation = proto.String("new")
		return true, nil
	}))
	require.NoError(t, <-deleted)

	_, err := store.GetMessage(updateTestID)
	require.ErrorIs(t, err, storage.ErrNotFound)
}
