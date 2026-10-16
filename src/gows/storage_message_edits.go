package gows

import (
	"errors"

	"github.com/avast/retry-go"
	"github.com/devlikeapro/gows/storage"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// errEditTargetNotReady: events are stored concurrently, so the edit can come before its original
var errEditTargetNotReady = errors.New("edited message has no content stored yet")

// handleMessageEdit writes the text of an edit into the stored original, reports whether it was an edit
func (st *StorageEventHandler) handleMessageEdit(event *events.Message) bool {
	targetID, edited, isEdit := st.extractMessageEdit(event)
	if !isEdit {
		return false
	}
	if targetID == "" || edited == nil {
		st.log.Debugf("Edit %v has no target id or no content, ignoring", event.Info.ID)
		return true
	}

	applied := false
	err := st.updateMessageWithRetries(targetID, func(stored *storage.StoredMessage) (bool, error) {
		applied = false
		if stored.Message == nil || stored.Message.Message == nil || !stored.IsReal {
			return false, errEditTargetNotReady
		}
		if !st.isEditFromAuthor(&stored.Info, &event.Info) {
			st.log.Warnf(
				"Ignoring edit %v of message %v: sent by %v in %v, the message is from %v in %v",
				event.Info.ID, targetID, event.Info.Sender, event.Info.Chat, stored.Info.Sender, stored.Info.Chat,
			)
			return false, nil
		}
		if stored.EditedAt.After(event.Info.Timestamp) {
			// a later edit is already applied
			return false, nil
		}
		if !applyTextEdit(stored.Message.Message, edited) {
			return false, nil
		}
		stored.EditedAt = event.Info.Timestamp
		applied = true
		return true, nil
	})

	switch {
	case err == nil && applied:
		st.log.Debugf("Applied edit %v to message %v", event.Info.ID, targetID)
	case err == nil:
		st.log.Debugf("Edit %v not applied to message %v", event.Info.ID, targetID)
	case errors.Is(err, storage.ErrNotFound), errors.Is(err, errEditTargetNotReady):
		st.log.Debugf("Edit %v not applied: message %v is not stored with its content", event.Info.ID, targetID)
	case errors.Is(err, storage.ErrStorageDisabled):
	default:
		st.log.Errorf("Error applying edit %v to message %v: %v", event.Info.ID, targetID, err)
	}
	return true
}

// extractMessageEdit returns the id of the edited message and its new content,
// from a MESSAGE_EDIT protocol message or a secret encrypted MESSAGE_EDIT
func (st *StorageEventHandler) extractMessageEdit(event *events.Message) (types.MessageID, *waE2E.Message, bool) {
	msg := event.Message
	if msg.GetEditedMessage().GetMessage() != nil {
		// newsletter edits keep the protocol message in a FutureProofMessage
		msg = msg.GetEditedMessage().GetMessage()
	}
	if protocol := msg.GetProtocolMessage(); protocol != nil {
		if protocol.GetType() != waE2E.ProtocolMessage_MESSAGE_EDIT {
			return "", nil, false
		}
		return protocol.GetKey().GetID(), protocol.GetEditedMessage(), true
	}
	secret := msg.GetSecretEncryptedMessage()
	if secret == nil || secret.GetSecretEncType() != waE2E.SecretEncryptedMessage_MESSAGE_EDIT {
		return "", nil, false
	}
	targetID := secret.GetTargetMessageKey().GetID()
	edited, err := st.decryptSecretMessage(event)
	if err != nil {
		st.log.Warnf("Failed to decrypt edit %v of message %v: %v", event.Info.ID, targetID, err)
		return targetID, nil, true
	}
	return targetID, edited, true
}

func (st *StorageEventHandler) decryptSecretMessage(event *events.Message) (*waE2E.Message, error) {
	if st.decryptSecret != nil {
		return st.decryptSecret(event)
	}
	if st.gows == nil || st.gows.Client == nil {
		return nil, errors.New("no client to decrypt it")
	}
	return st.gows.DecryptSecretEncryptedMessage(st.gows.Context, event)
}

// isEditFromAuthor: only the author can edit a message and whatsmeow doesn't check it,
// so without this any group member could rewrite someone else's stored message
func (st *StorageEventHandler) isEditFromAuthor(original, edit *types.MessageInfo) bool {
	if (original.IsGroup || edit.IsGroup) && original.Chat.ToNonAD() != edit.Chat.ToNonAD() {
		return false
	}
	if original.IsFromMe || edit.IsFromMe {
		return original.IsFromMe && edit.IsFromMe
	}
	authors := st.userJIDs(original)
	for jid := range st.userJIDs(edit) {
		if authors[jid] {
			return true
		}
	}
	return false
}

// userJIDs returns the sender of a message as LID and phone number, when known
func (st *StorageEventHandler) userJIDs(info *types.MessageInfo) map[types.JID]bool {
	jids := make(map[types.JID]bool, 3)
	for _, jid := range []types.JID{info.Sender, info.SenderAlt} {
		if jid.IsEmpty() {
			continue
		}
		jids[jid.ToNonAD()] = true
		if jid.Server == types.HiddenUserServer && st.gows != nil && st.gows.Client != nil &&
			st.gows.Store != nil && st.gows.Store.LIDs != nil {
			pn, err := st.gows.Store.LIDs.GetPNForLID(st.gows.Context, jid)
			if err == nil && !pn.IsEmpty() {
				jids[pn.ToNonAD()] = true
			}
		}
	}
	return jids
}

// updateMessageWithRetries retries while the message is not stored (with its content) yet
func (st *StorageEventHandler) updateMessageWithRetries(id types.MessageID, update func(msg *storage.StoredMessage) (bool, error)) error {
	return retry.Do(
		func() error {
			return st.storage.Messages.UpdateMessage(id, update)
		},
		retry.Attempts(6),
		retry.LastErrorOnly(true),
		retry.RetryIf(func(err error) bool {
			return !errors.Is(err, storage.ErrStorageDisabled)
		}),
	)
}
