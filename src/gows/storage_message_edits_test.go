package gows

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	gowsLog "github.com/devlikeapro/gows/log"
	"github.com/devlikeapro/gows/storage"
	"github.com/devlikeapro/gows/storage/sqlstorage"
	"github.com/stretchr/testify/require"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

const (
	editGroupJID    = "120363000000000002@g.us"
	editAuthorJID   = "100000000000001:1@lid"
	editOtherJID    = "100000000000002:3@lid"
	editOriginalID  = "3EB00000000000000000A1"
	editEventID     = "3EB00000000000000000E1"
	editOriginalTxt = "See you at 10"
	editNewTxt      = "See you at 11"
)

var editOriginalTime = time.Unix(1790000000, 0)

// newEditTestHandler returns a storage event handler on a real SQLite store in a temp dir.
func newEditTestHandler(t *testing.T) (*StorageEventHandler, *sqlstorage.SqlMessageStore) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gows.db")
	container, err := sqlstorage.New("sqlite3", "file:"+path+"?_foreign_keys=on", waLog.Noop)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Close() })
	store := container.NewMessageStorage()
	st := &StorageEventHandler{
		log:     gowsLog.Stdout("test", "ERROR", false),
		storage: &storage.Storage{Messages: store},
	}
	return st, store
}

func editSource(t *testing.T, sender string) types.MessageSource {
	t.Helper()
	chat, err := types.ParseJID(editGroupJID)
	require.NoError(t, err)
	senderJID, err := types.ParseJID(sender)
	require.NoError(t, err)
	return types.MessageSource{Chat: chat, Sender: senderJID, IsGroup: true}
}

func editIncoming(t *testing.T, id, sender string, at time.Time, msg *waE2E.Message) *events.Message {
	t.Helper()
	return &events.Message{
		Info:       types.MessageInfo{ID: id, Timestamp: at, MessageSource: editSource(t, sender)},
		Message:    msg,
		RawMessage: msg,
	}
}

func editTextMessage(text string) *waE2E.Message {
	return &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String(text)}}
}

// editProtocol is an edit the way it reaches gows: a MESSAGE_EDIT protocol message
// stored under its own id, pointing at the original through its key.
func editProtocol(t *testing.T, id, targetID, sender string, at time.Time, content *waE2E.Message) *events.Message {
	t.Helper()
	msg := &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		Key: &waCommon.MessageKey{
			RemoteJID: proto.String(editGroupJID),
			ID:        proto.String(targetID),
		},
		Type:          waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
		EditedMessage: content,
		TimestampMS:   proto.Int64(at.UnixMilli()),
	}}
	evt := editIncoming(t, id, sender, at, msg)
	evt.Info.Edit = types.EditAttributeMessageEdit
	return evt
}

func storedEditTarget(t *testing.T, store *sqlstorage.SqlMessageStore, id string) *storage.StoredMessage {
	t.Helper()
	msg, err := store.GetMessage(id)
	require.NoError(t, err)
	require.NotNil(t, msg.Message.Message)
	return msg
}

func listedIDs(t *testing.T, store *sqlstorage.SqlMessageStore) []string {
	t.Helper()
	chat, err := types.ParseJID(editGroupJID)
	require.NoError(t, err)
	msgs, err := store.GetChatMessages(chat, storage.MessageFilter{}, storage.Pagination{Limit: 50}, false)
	require.NoError(t, err)
	ids := make([]string, 0, len(msgs))
	for _, m := range msgs {
		ids = append(ids, m.Info.ID)
	}
	return ids
}

func TestEditReceivedAfterOriginalReplacesItsText(t *testing.T) {
	st, store := newEditTestHandler(t)
	st.processEvent(editIncoming(t, editOriginalID, editAuthorJID, editOriginalTime, editTextMessage(editOriginalTxt)))

	editAt := editOriginalTime.Add(3 * time.Minute)
	st.processEvent(editProtocol(t, editEventID, editOriginalID, editAuthorJID, editAt, editTextMessage(editNewTxt)))

	stored := storedEditTarget(t, store, editOriginalID)
	require.Equal(t, editNewTxt, stored.Message.Message.GetExtendedTextMessage().GetText())
	require.True(t, stored.EditedAt.Equal(editAt))
	require.True(t, stored.IsReal)
	// The raw message is what gets re-sent on a retry receipt: it stays the original.
	require.Equal(t, editOriginalTxt, stored.Message.RawMessage.GetExtendedTextMessage().GetText())
	// The original is still listed, the edit event is not.
	require.Equal(t, []string{editOriginalID}, listedIDs(t, store))
	// The edit event is still stored under its own id.
	edit := storedEditTarget(t, store, editEventID)
	require.Equal(t, waE2E.ProtocolMessage_MESSAGE_EDIT, edit.Message.Message.GetProtocolMessage().GetType())
}

func TestEditHandledBeforeOriginalIsAppliedOnceOriginalIsStored(t *testing.T) {
	st, store := newEditTestHandler(t)

	done := make(chan struct{})
	go func() {
		defer close(done)
		st.processEvent(editProtocol(t, editEventID, editOriginalID, editAuthorJID, editOriginalTime.Add(time.Minute), editTextMessage(editNewTxt)))
	}()
	time.Sleep(300 * time.Millisecond)
	st.processEvent(editIncoming(t, editOriginalID, editAuthorJID, editOriginalTime, editTextMessage(editOriginalTxt)))
	<-done

	stored := storedEditTarget(t, store, editOriginalID)
	require.Equal(t, editNewTxt, stored.Message.Message.GetExtendedTextMessage().GetText())
}

func TestEditWaitsForContentWhenOnlySenderKeyPartIsStored(t *testing.T) {
	st, store := newEditTestHandler(t)
	senderKeyPart := &waE2E.Message{
		SenderKeyDistributionMessage: &waE2E.SenderKeyDistributionMessage{
			GroupID:                             proto.String(editGroupJID),
			AxolotlSenderKeyDistributionMessage: []byte{0x33, 0x08, 0x01},
		},
	}
	st.processEvent(editIncoming(t, editOriginalID, editAuthorJID, editOriginalTime, senderKeyPart))

	done := make(chan struct{})
	go func() {
		defer close(done)
		st.processEvent(editProtocol(t, editEventID, editOriginalID, editAuthorJID, editOriginalTime.Add(time.Minute), editTextMessage(editNewTxt)))
	}()
	time.Sleep(300 * time.Millisecond)
	st.processEvent(editIncoming(t, editOriginalID, editAuthorJID, editOriginalTime, editTextMessage(editOriginalTxt)))
	<-done

	stored := storedEditTarget(t, store, editOriginalID)
	require.Equal(t, editNewTxt, stored.Message.Message.GetExtendedTextMessage().GetText())
}

func TestEditOfCaptionKeepsTheMedia(t *testing.T) {
	st, store := newEditTestHandler(t)
	image := &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
		URL:           proto.String("https://mmg.whatsapp.net/o1/v/t24/f2/m237/example"),
		DirectPath:    proto.String("/o1/v/t24/f2/m237/example"),
		Mimetype:      proto.String("image/jpeg"),
		Caption:       proto.String("Photo 1"),
		MediaKey:      []byte{0x01, 0x02, 0x03},
		FileSHA256:    []byte{0x04, 0x05},
		FileEncSHA256: []byte{0x06, 0x07},
		FileLength:    proto.Uint64(74147),
		Width:         proto.Uint32(720),
		Height:        proto.Uint32(1000),
	}}
	st.processEvent(editIncoming(t, editOriginalID, editAuthorJID, editOriginalTime, image))

	newCaption := &waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: proto.String("Photo 2")}}
	st.processEvent(editProtocol(t, editEventID, editOriginalID, editAuthorJID, editOriginalTime.Add(time.Minute), newCaption))

	stored := storedEditTarget(t, store, editOriginalID).Message.Message.GetImageMessage()
	require.NotNil(t, stored)
	require.Equal(t, "Photo 2", stored.GetCaption())
	require.Equal(t, "https://mmg.whatsapp.net/o1/v/t24/f2/m237/example", stored.GetURL())
	require.Equal(t, "/o1/v/t24/f2/m237/example", stored.GetDirectPath())
	require.Equal(t, "image/jpeg", stored.GetMimetype())
	require.Equal(t, []byte{0x01, 0x02, 0x03}, stored.GetMediaKey())
	require.Equal(t, []byte{0x04, 0x05}, stored.GetFileSHA256())
	require.Equal(t, []byte{0x06, 0x07}, stored.GetFileEncSHA256())
	require.Equal(t, uint64(74147), stored.GetFileLength())
	require.Equal(t, []string{editOriginalID}, listedIDs(t, store))
}

func TestOlderEditDoesNotReplaceNewerOne(t *testing.T) {
	st, store := newEditTestHandler(t)
	st.processEvent(editIncoming(t, editOriginalID, editAuthorJID, editOriginalTime, editTextMessage(editOriginalTxt)))

	st.processEvent(editProtocol(t, "3EB0000000000000000002", editOriginalID, editAuthorJID, editOriginalTime.Add(2*time.Minute), editTextMessage("See you at 12")))
	st.processEvent(editProtocol(t, "3EB0000000000000000001", editOriginalID, editAuthorJID, editOriginalTime.Add(time.Minute), editTextMessage(editNewTxt)))

	stored := storedEditTarget(t, store, editOriginalID)
	require.Equal(t, "See you at 12", stored.Message.Message.GetExtendedTextMessage().GetText())
}

func TestEditFromSomeoneElseIsIgnored(t *testing.T) {
	st, store := newEditTestHandler(t)
	st.processEvent(editIncoming(t, editOriginalID, editAuthorJID, editOriginalTime, editTextMessage(editOriginalTxt)))

	// Another member of the group
	st.processEvent(editProtocol(t, editEventID, editOriginalID, editOtherJID, editOriginalTime.Add(time.Minute), editTextMessage(editNewTxt)))
	require.Equal(t, editOriginalTxt, storedEditTarget(t, store, editOriginalID).Message.Message.GetExtendedTextMessage().GetText())

	// An edit sent by us cannot target a message we did not send either
	fromMe := editProtocol(t, "3EB0000000000000000003", editOriginalID, editOtherJID, editOriginalTime.Add(time.Minute), editTextMessage(editNewTxt))
	fromMe.Info.IsFromMe = true
	st.processEvent(fromMe)
	require.Equal(t, editOriginalTxt, storedEditTarget(t, store, editOriginalID).Message.Message.GetExtendedTextMessage().GetText())
}

// The author can be addressed by LID in one message and by phone number in the other
func TestEditFromAuthorWithOtherAddressingIsApplied(t *testing.T) {
	st, store := newEditTestHandler(t)
	original := editIncoming(t, editOriginalID, "15550100002@s.whatsapp.net", editOriginalTime, editTextMessage(editOriginalTxt))
	original.Info.SenderAlt = types.NewJID("100000000000003", types.HiddenUserServer)
	st.processEvent(original)

	st.processEvent(editProtocol(t, editEventID, editOriginalID, "100000000000003:2@lid", editOriginalTime.Add(time.Minute), editTextMessage(editNewTxt)))

	require.Equal(t, editNewTxt, storedEditTarget(t, store, editOriginalID).Message.Message.GetExtendedTextMessage().GetText())
}

func TestOwnEditOfOwnMessageIsApplied(t *testing.T) {
	st, store := newEditTestHandler(t)
	original := editIncoming(t, editOriginalID, "15550100001:1@s.whatsapp.net", editOriginalTime, editTextMessage(editOriginalTxt))
	original.Info.IsFromMe = true
	st.processEvent(original)

	edit := editProtocol(t, editEventID, editOriginalID, "15550100001:1@s.whatsapp.net", editOriginalTime.Add(time.Minute), editTextMessage(editNewTxt))
	edit.Info.IsFromMe = true
	st.processEvent(edit)

	require.Equal(t, editNewTxt, storedEditTarget(t, store, editOriginalID).Message.Message.GetExtendedTextMessage().GetText())
}

func TestSecretEncryptedEditIsApplied(t *testing.T) {
	st, store := newEditTestHandler(t)
	st.processEvent(editIncoming(t, editOriginalID, editAuthorJID, editOriginalTime, editTextMessage(editOriginalTxt)))

	secret := &waE2E.Message{SecretEncryptedMessage: &waE2E.SecretEncryptedMessage{
		TargetMessageKey: &waCommon.MessageKey{RemoteJID: proto.String(editGroupJID), ID: proto.String(editOriginalID)},
		EncPayload:       []byte{0x0a, 0x0b},
		EncIV:            []byte{0x0c},
		SecretEncType:    waE2E.SecretEncryptedMessage_MESSAGE_EDIT.Enum(),
	}}

	// Decryption fails (no message secret): nothing changes
	st.decryptSecret = func(*events.Message) (*waE2E.Message, error) {
		return nil, errors.New("original message secret not found")
	}
	st.processEvent(editIncoming(t, editEventID, editAuthorJID, editOriginalTime.Add(time.Minute), secret))
	require.Equal(t, editOriginalTxt, storedEditTarget(t, store, editOriginalID).Message.Message.GetExtendedTextMessage().GetText())

	st.decryptSecret = func(evt *events.Message) (*waE2E.Message, error) {
		require.Equal(t, editOriginalID, evt.Message.GetSecretEncryptedMessage().GetTargetMessageKey().GetID())
		return &waE2E.Message{Conversation: proto.String(editNewTxt)}, nil
	}
	st.processEvent(editIncoming(t, editEventID, editAuthorJID, editOriginalTime.Add(time.Minute), secret))
	require.Equal(t, editNewTxt, storedEditTarget(t, store, editOriginalID).Message.Message.GetExtendedTextMessage().GetText())
}

// A receipt rewrites the whole stored message to change its status. Handled at the
// same time as an edit, it must not write back the text from before the edit.
func TestReceiptAndEditHandledTogetherKeepBothChanges(t *testing.T) {
	st, store := newEditTestHandler(t)
	chat, err := types.ParseJID(editGroupJID)
	require.NoError(t, err)

	for i := 0; i < 200; i++ {
		id := fmt.Sprintf("3EB0%018d", i)
		original := editIncoming(t, id, "15550100001:1@s.whatsapp.net", editOriginalTime, editTextMessage(editOriginalTxt))
		original.Info.IsFromMe = true
		st.processEvent(original)

		edit := editProtocol(t, fmt.Sprintf("3EB1%018d", i), id, "15550100001:1@s.whatsapp.net", editOriginalTime.Add(time.Minute), editTextMessage(editNewTxt))
		edit.Info.IsFromMe = true
		receipt := &events.Receipt{
			MessageSource: types.MessageSource{Chat: chat, IsGroup: true},
			MessageIDs:    []types.MessageID{id},
			Type:          types.ReceiptTypeRead,
		}

		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); st.processEvent(receipt) }()
		go func() { defer wg.Done(); st.processEvent(edit) }()
		wg.Wait()

		stored := storedEditTarget(t, store, id)
		require.Equal(t, editNewTxt, stored.Message.Message.GetExtendedTextMessage().GetText(), "message %d lost its edit", i)
		require.Equal(t, storage.StatusRead, *stored.Status, "message %d lost its receipt", i)
	}
}
