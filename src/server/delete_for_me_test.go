package server

import (
	"errors"
	"fmt"
	"testing"

	"github.com/devlikeapro/gows/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	dfmChat  = "15550100001@s.whatsapp.net"
	dfmLID   = "111111111111111@lid"
	dfmOther = "15550100002@s.whatsapp.net"
)

func dfmStore(messages map[string]string) func(types.MessageID) (*storage.StoredMessage, error) {
	return func(id types.MessageID) (*storage.StoredMessage, error) {
		chat, ok := messages[id]
		if !ok {
			return nil, storage.ErrNotFound
		}
		jid, err := types.ParseJID(chat)
		if err != nil {
			return nil, err
		}
		return &storage.StoredMessage{Message: &events.Message{Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: jid},
			ID:            id,
		}}}, nil
	}
}

// dfmSameChat treats dfmLID as the LID of dfmChat.
func dfmSameChat(a, b types.JID) (bool, error) {
	if a == b {
		return true, nil
	}
	pair := map[string]bool{a.String() + " " + b.String(): true}
	return pair[dfmLID+" "+dfmChat] || pair[dfmChat+" "+dfmLID], nil
}

func TestMessagesToDeleteForMe(t *testing.T) {
	store := dfmStore(map[string]string{"in-chat": dfmChat, "under-lid": dfmLID})

	messages, err := messagesToDeleteForMe(dfmChat, []string{"in-chat", "under-lid"}, store, dfmSameChat)

	require.NoError(t, err)
	require.Len(t, messages, 2)
	assert.Equal(t, dfmChat, messages[0].Chat.String())
	// The message keeps the address it is stored under.
	assert.Equal(t, dfmLID, messages[1].Chat.String())
}

func TestMessagesToDeleteForMeRejects(t *testing.T) {
	store := dfmStore(map[string]string{"in-chat": dfmChat, "elsewhere": dfmOther})
	tooMany := make([]string, maxDeleteMessagesForMe+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("id-%d", i)
	}
	cases := []struct {
		name  string
		chat  string
		ids   []string
		store func(types.MessageID) (*storage.StoredMessage, error)
		code  codes.Code
	}{
		{name: "no ids", chat: dfmChat, code: codes.InvalidArgument},
		{name: "too many ids", chat: dfmChat, ids: tooMany, code: codes.InvalidArgument},
		{name: "invalid chat", chat: "15550100001", ids: []string{"in-chat"}, code: codes.InvalidArgument},
		{name: "empty chat", chat: "", ids: []string{"in-chat"}, code: codes.InvalidArgument},
		{name: "status", chat: "status@broadcast", ids: []string{"in-chat"}, code: codes.InvalidArgument},
		{name: "broadcast list", chat: "1700000000@broadcast", ids: []string{"in-chat"}, code: codes.InvalidArgument},
		{name: "newsletter", chat: "120363000000000002@newsletter", ids: []string{"in-chat"}, code: codes.InvalidArgument},
		{name: "unknown message", chat: dfmChat, ids: []string{"in-chat", "missing"}, code: codes.NotFound},
		{name: "message of another chat", chat: dfmChat, ids: []string{"in-chat", "elsewhere"}, code: codes.InvalidArgument},
		{
			name: "storage disabled", chat: dfmChat, ids: []string{"in-chat"}, code: codes.FailedPrecondition,
			store: func(types.MessageID) (*storage.StoredMessage, error) {
				return nil, storage.StorageDisabled("messages")
			},
		},
		{
			name: "storage error", chat: dfmChat, ids: []string{"in-chat"}, code: codes.Internal,
			store: func(types.MessageID) (*storage.StoredMessage, error) {
				return nil, errors.New("database is locked")
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lookup := c.store
			if lookup == nil {
				lookup = store
			}
			messages, err := messagesToDeleteForMe(c.chat, c.ids, lookup, dfmSameChat)
			assert.Nil(t, messages)
			assert.Equal(t, c.code, status.Code(err), "error: %v", err)
		})
	}
}
