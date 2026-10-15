package gows

import (
	"testing"

	gowsLog "github.com/devlikeapro/gows/log"
	"github.com/devlikeapro/gows/storage"
	"github.com/devlikeapro/gows/storage/noop"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// spyMessageStorage records the messages we asked to delete.
type spyMessageStorage struct {
	*noop.MessageStorage
	deleted []types.MessageID
}

func (s *spyMessageStorage) DeleteMessage(id types.MessageID) error {
	s.deleted = append(s.deleted, id)
	return nil
}

// spyLabelStorage records the labels we upserted and deleted.
type spyLabelStorage struct {
	*noop.LabelStorage
	upserted []*storage.Label
	deleted  []string
}

func (s *spyLabelStorage) UpsertLabel(label *storage.Label) error {
	s.upserted = append(s.upserted, label)
	return nil
}

func (s *spyLabelStorage) DeleteLabel(id string) error {
	s.deleted = append(s.deleted, id)
	return nil
}

// spyLabelAssociationStorage records the association calls in order.
type spyLabelAssociationStorage struct {
	*noop.LabelAssociationStorage
	ops []string
}

func (s *spyLabelAssociationStorage) AddAssociation(jid types.JID, labelID string) error {
	s.ops = append(s.ops, "add "+labelID+" "+jid.String())
	return nil
}

func (s *spyLabelAssociationStorage) RemoveAssociation(jid types.JID, labelID string) error {
	s.ops = append(s.ops, "remove "+labelID+" "+jid.String())
	return nil
}

func (s *spyLabelAssociationStorage) RemoveAssociationsByLabelID(labelID string) error {
	s.ops = append(s.ops, "removeByLabel "+labelID)
	return nil
}

func newTestHandler() (*StorageEventHandler, *spyMessageStorage, *spyLabelStorage, *spyLabelAssociationStorage) {
	messages := &spyMessageStorage{MessageStorage: noop.NewMessageStorage()}
	labels := &spyLabelStorage{LabelStorage: noop.NewLabelStorage()}
	associations := &spyLabelAssociationStorage{LabelAssociationStorage: noop.NewLabelAssociationStorage()}
	st := &StorageEventHandler{
		log: gowsLog.Stdout("test", "ERROR", false),
		storage: &storage.Storage{
			Messages:          messages,
			Labels:            labels,
			LabelAssociations: associations,
		},
	}
	return st, messages, labels, associations
}

func labelChatEvent(labelID string, jid types.JID, labeled bool) *events.LabelAssociationChat {
	return &events.LabelAssociationChat{
		JID:     jid,
		LabelID: labelID,
		Action:  &waSyncAction.LabelAssociationAction{Labeled: &labeled},
	}
}

func conversationMessage(text string) *events.Message {
	return &events.Message{
		Info:    types.MessageInfo{ID: "id-1"},
		Message: &waE2E.Message{Conversation: &text},
	}
}

func TestHandleMessageEventNilMessage(t *testing.T) {
	st, messages, _, _ := newTestHandler()

	// events.Message.Message is nil for messages we could not decrypt
	st.handleMessageEvent(&events.Message{Info: types.MessageInfo{ID: "id-1"}})

	if len(messages.deleted) != 0 {
		t.Fatalf("expected no deletes, got %v", messages.deleted)
	}
}

// ProtocolMessage_REVOKE is the zero value of the enum, so a message without a
// protocol message must never be mistaken for a revoke.
func TestHandleMessageEventPlainMessageIsNotRevoke(t *testing.T) {
	st, messages, _, _ := newTestHandler()

	st.handleMessageEvent(conversationMessage("hello"))

	if len(messages.deleted) != 0 {
		t.Fatalf("a plain message must not delete anything, got %v", messages.deleted)
	}
}

func TestHandleMessageEventRevokeWithoutKey(t *testing.T) {
	st, messages, _, _ := newTestHandler()

	revoke := waE2E.ProtocolMessage_REVOKE
	event := &events.Message{
		Info: types.MessageInfo{ID: "id-1"},
		Message: &waE2E.Message{
			ProtocolMessage: &waE2E.ProtocolMessage{Type: &revoke},
		},
	}
	st.handleMessageEvent(event)

	if len(messages.deleted) != 0 {
		t.Fatalf("a revoke without a key must not delete anything, got %v", messages.deleted)
	}
}

func TestHandleMessageEventRevoke(t *testing.T) {
	st, _, _, _ := newTestHandler()

	revoke := waE2E.ProtocolMessage_REVOKE
	id := "revoked-id"
	author := types.NewJID("15550100001", types.DefaultUserServer)
	source := types.MessageSource{Chat: author, Sender: author}
	messages := &revokeMessageStorage{stored: map[types.MessageID]*storage.StoredMessage{
		id: {Message: &events.Message{Info: types.MessageInfo{ID: id, MessageSource: source}}},
	}}
	st.storage.Messages = messages
	event := &events.Message{
		Info: types.MessageInfo{ID: "id-1", MessageSource: source},
		Message: &waE2E.Message{
			ProtocolMessage: &waE2E.ProtocolMessage{
				Type: &revoke,
				Key:  &waCommon.MessageKey{ID: &id},
			},
		},
	}
	st.handleMessageEvent(event)

	if len(messages.deleted) != 1 || messages.deleted[0] != id {
		t.Fatalf("expected to delete %v, got %v", id, messages.deleted)
	}
}

// A protocol message with no type used to dereference a nil *ProtocolMessage_Type.
func TestExtractEphemeralSettingsFromProtocolMessageWithoutType(t *testing.T) {
	setting := ExtractEphemeralSettingsFromProtocolMessage(
		types.MessageInfo{},
		&waE2E.ProtocolMessage{},
	)
	if setting != nil {
		t.Fatalf("expected no ephemeral setting, got %v", setting)
	}
}

// LabelEditAction has optional Name/Color, both used to be dereferenced blindly.
func TestHandleLabelEditWithoutNameAndColor(t *testing.T) {
	st, _, labels, _ := newTestHandler()

	st.handleLabelEdit(&events.LabelEdit{
		LabelID: "label-1",
		Action:  &waSyncAction.LabelEditAction{},
	})

	if len(labels.upserted) != 1 {
		t.Fatalf("expected the label to be upserted, got %v", labels.upserted)
	}
	if labels.upserted[0].Name != "" || labels.upserted[0].Color != 0 {
		t.Fatalf("expected empty name and zero color, got %v", labels.upserted[0])
	}
	if labels.upserted[0].Type != nil || labels.upserted[0].IsActive != nil {
		t.Fatalf("expected no type and no isActive, got %v", labels.upserted[0])
	}
}

// WhatsApp lists (Unread, Favourites, ...) live in the same collection - keep their type, the API tells them apart
func TestHandleLabelEditKeepsListType(t *testing.T) {
	st, _, labels, _ := newTestHandler()

	name := "Unread"
	listType := waSyncAction.LabelEditAction_UNREAD
	active := true
	st.handleLabelEdit(&events.LabelEdit{
		LabelID: "41",
		Action:  &waSyncAction.LabelEditAction{Name: &name, Type: &listType, IsActive: &active},
	})

	if len(labels.upserted) != 1 {
		t.Fatalf("expected the list to be upserted, got %v", labels.upserted)
	}
	label := labels.upserted[0]
	if label.Type == nil || *label.Type != 1 {
		t.Fatalf("expected type 1 (UNREAD), got %v", label.Type)
	}
	if label.IsActive == nil || !*label.IsActive {
		t.Fatalf("expected isActive true, got %v", label.IsActive)
	}
}

func TestHandleLabelEditDeleted(t *testing.T) {
	st, _, labels, associations := newTestHandler()

	deleted := true
	st.handleLabelEdit(&events.LabelEdit{
		LabelID: "label-1",
		Action:  &waSyncAction.LabelEditAction{Deleted: &deleted},
	})

	if len(labels.upserted) != 0 {
		t.Fatalf("a deleted label must not be upserted, got %v", labels.upserted)
	}
	// No FK cascade in the DB anymore - the associations go first, then the label
	if len(associations.ops) != 1 || associations.ops[0] != "removeByLabel label-1" {
		t.Fatalf("expected the label associations to be removed, got %v", associations.ops)
	}
	if len(labels.deleted) != 1 || labels.deleted[0] != "label-1" {
		t.Fatalf("expected the label to be deleted, got %v", labels.deleted)
	}
}

// Label events run one by one in handleEvent, so the last change per label/chat wins (#2290).
func TestHandleEventLabelAssociationKeepsOrder(t *testing.T) {
	st, _, _, associations := newTestHandler()
	jid := types.NewJID("100000000000001", types.HiddenUserServer)

	st.handleEvent(labelChatEvent("label-1", jid, true))
	st.handleEvent(labelChatEvent("label-1", jid, false))
	st.handleEvent(labelChatEvent("label-1", jid, true))
	st.handleEvent(labelChatEvent("label-1", jid, false))

	expected := []string{
		"add label-1 " + jid.String(),
		"remove label-1 " + jid.String(),
		"add label-1 " + jid.String(),
		"remove label-1 " + jid.String(),
	}
	if len(associations.ops) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, associations.ops)
	}
	for i := range expected {
		if associations.ops[i] != expected[i] {
			t.Fatalf("expected %v, got %v", expected, associations.ops)
		}
	}
}

// The label can arrive after the association in a sync - store the association anyway, no waiting.
func TestHandleLabelAssociationChatWithoutLabel(t *testing.T) {
	st, _, _, associations := newTestHandler()
	jid := types.NewJID("100000000000001", types.HiddenUserServer)

	st.handleLabelAssociationChat(labelChatEvent("label-404", jid, true))

	if len(associations.ops) != 1 || associations.ops[0] != "add label-404 "+jid.String() {
		t.Fatalf("expected the association to be added, got %v", associations.ops)
	}
}

func TestHandleHistorySyncWithoutData(t *testing.T) {
	st, _, _, _ := newTestHandler()

	// Must not dereference a nil event.Data
	st.handleHistorySync(&events.HistorySync{})
}
