package gows

import (
	"testing"
	"time"

	"github.com/devlikeapro/gows/storage"
	"github.com/devlikeapro/gows/storage/noop"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// fakeMessageStorage knows a few messages and counts the reads.
type fakeMessageStorage struct {
	*noop.MessageStorage
	messages map[types.MessageID]*storage.StoredMessage
	reads    int
}

func (s *fakeMessageStorage) GetMessage(id types.MessageID) (*storage.StoredMessage, error) {
	s.reads++
	if msg, ok := s.messages[id]; ok {
		return msg, nil
	}
	return nil, storage.ErrNotFound
}

func newRetryInfoTestGoWS(msgs ...*storage.StoredMessage) (*GoWS, *fakeMessageStorage) {
	messages := &fakeMessageStorage{
		MessageStorage: noop.NewMessageStorage(),
		messages:       map[types.MessageID]*storage.StoredMessage{},
	}
	for _, msg := range msgs {
		messages.messages[msg.Info.ID] = msg
	}
	return &GoWS{Storage: &storage.Storage{Messages: messages}}, messages
}

func retryInfoTestJID(t *testing.T, s string) types.JID {
	t.Helper()
	jid, err := types.ParseJID(s)
	if err != nil {
		t.Fatal(err)
	}
	return jid
}

// requestInfo is the Info DownloadMedia builds from the request when it doesn't read storage.
func requestInfo(t *testing.T, chat string, id types.MessageID) *types.MessageInfo {
	jid := retryInfoTestJID(t, chat)
	return &types.MessageInfo{
		ID:            id,
		MessageSource: types.MessageSource{Chat: jid, IsGroup: jid.Server == types.GroupServer},
	}
}

func storedImage(info types.MessageInfo) *storage.StoredMessage {
	msg := &waE2E.Message{ImageMessage: &waE2E.ImageMessage{DirectPath: proto.String("/v/t62.7118-24/example")}}
	return &storage.StoredMessage{
		Message: &events.Message{Info: info, Message: msg, RawMessage: msg},
		IsReal:  true,
	}
}

func TestMediaRetryInfoTakesTheSenderFromStorage(t *testing.T) {
	cases := map[string]types.MessageInfo{
		"group": {ID: "3EB00000000000000000B1", Timestamp: time.Unix(1790000000, 0), MessageSource: types.MessageSource{
			Chat:    retryInfoTestJID(t, "120363000000000001@g.us"),
			Sender:  retryInfoTestJID(t, "100000000000001@lid"),
			IsGroup: true,
		}},
		"group, from me": {ID: "3EB00000000000000000B2", MessageSource: types.MessageSource{
			Chat:     retryInfoTestJID(t, "120363000000000001@g.us"),
			Sender:   retryInfoTestJID(t, "15550100001@s.whatsapp.net"),
			IsFromMe: true,
			IsGroup:  true,
		}},
		"direct, from me": {ID: "3EB00000000000000000B3", MessageSource: types.MessageSource{
			Chat:     retryInfoTestJID(t, "15550100002@s.whatsapp.net"),
			Sender:   retryInfoTestJID(t, "15550100001@s.whatsapp.net"),
			IsFromMe: true,
		}},
	}
	for name, stored := range cases {
		t.Run(name, func(t *testing.T) {
			g, _ := newRetryInfoTestGoWS(storedImage(stored))

			got := g.mediaRetryInfo(requestInfo(t, stored.Chat.String(), stored.ID))

			if got.Sender != stored.Sender || got.IsFromMe != stored.IsFromMe || got.IsGroup != stored.IsGroup || got.Chat != stored.Chat {
				t.Fatalf("expected the stored Info %+v, got %+v", stored, got)
			}
		})
	}
}

func TestMediaRetryInfoKeepsTheRequestInfoWithoutStoredMessage(t *testing.T) {
	g, _ := newRetryInfoTestGoWS()
	info := requestInfo(t, "120363000000000001@g.us", "3EB00000000000000000B4")
	if got := g.mediaRetryInfo(info); got != info {
		t.Fatalf("expected the request Info, got %+v", got)
	}

	disabled := &GoWS{Storage: &storage.Storage{Messages: noop.NewMessageStorage()}}
	if got := disabled.mediaRetryInfo(info); got != info {
		t.Fatalf("expected the request Info with storage disabled, got %+v", got)
	}

	noStorage := &GoWS{}
	if got := noStorage.mediaRetryInfo(info); got != info {
		t.Fatalf("expected the request Info without storage, got %+v", got)
	}
}

// An Info that already comes from storage is used as is.
func TestMediaRetryInfoDoesNotReadStorageForAFullInfo(t *testing.T) {
	stored := types.MessageInfo{ID: "3EB00000000000000000B5", MessageSource: types.MessageSource{
		Chat:    retryInfoTestJID(t, "120363000000000001@g.us"),
		Sender:  retryInfoTestJID(t, "100000000000001@lid"),
		IsGroup: true,
	}}
	g, messages := newRetryInfoTestGoWS(storedImage(stored))
	info := stored
	if got := g.mediaRetryInfo(&info); got != &info {
		t.Fatalf("expected the given Info, got %+v", got)
	}
	if messages.reads != 0 {
		t.Fatalf("storage should not be read, got %d reads", messages.reads)
	}
}
