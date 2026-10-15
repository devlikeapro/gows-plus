package gows

import (
	"errors"

	"github.com/devlikeapro/gows/storage"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// handleRevoke deletes a message revoked for everyone. whatsmeow doesn't check the revoke
// and its key is set by the sender, so it must come from the same chat
// and from the author, a group admin or us.
func (st *StorageEventHandler) handleRevoke(event *events.Message, id types.MessageID) {
	stored, err := st.storage.Messages.GetMessage(id)
	if errors.Is(err, storage.ErrNotFound) || errors.Is(err, storage.ErrStorageDisabled) {
		return
	}
	if err != nil {
		st.log.Errorf("Error getting revoked message %v: %v", id, err)
		return
	}
	if stored == nil || stored.Message == nil {
		return
	}
	if !st.canRevoke(&event.Info, &stored.Info) {
		st.log.Warnf("Ignoring revoke %v of message %v: sent by %v in %v, message from %v in %v",
			event.Info.ID, id, event.Info.Sender, event.Info.Chat, stored.Info.Sender, stored.Info.Chat)
		return
	}
	err = st.storage.Messages.DeleteMessage(id)
	if err != nil {
		st.log.Errorf("Error deleting message %v: %v", id, err)
	}
}

func (st *StorageEventHandler) canRevoke(revoke, target *types.MessageInfo) bool {
	if !st.sameJID(revoke.Chat, target.Chat) {
		return false
	}
	switch {
	case revoke.IsFromMe:
		// This or another of our devices
		return true
	case revoke.Chat.Server == types.NewsletterServer:
		// Only the channel admins can send to a channel
		return true
	case st.sameJID(revoke.Sender, target.Sender):
		return true
	case revoke.Chat.Server == types.GroupServer:
		return st.isGroupAdmin(revoke.Chat, revoke.Sender)
	}
	return false
}

// sameJID compares JIDs without the device, a LID equals its known phone number
func (st *StorageEventHandler) sameJID(a, b types.JID) bool {
	a, b = a.ToNonAD(), b.ToNonAD()
	if a.IsEmpty() || b.IsEmpty() {
		return false
	}
	if a == b {
		return true
	}
	if a.Server == types.DefaultUserServer {
		a, b = b, a
	}
	if a.Server != types.HiddenUserServer || b.Server != types.DefaultUserServer {
		return false
	}
	if st.gows == nil || st.gows.Client == nil || st.gows.Store == nil || st.gows.Store.LIDs == nil {
		return false
	}
	pn, err := st.gows.Store.LIDs.GetPNForLID(st.gows.Context, a)
	if err != nil {
		st.log.Warnf("Failed to get PN for %v: %v", a, err)
		return false
	}
	return !pn.IsEmpty() && pn.User == b.User && st.gows.validLIDPNPair(a, pn)
}

// isGroupAdmin uses the stored group, unknown is not an admin
func (st *StorageEventHandler) isGroupAdmin(group, user types.JID) bool {
	if st.storage.Groups == nil {
		return false
	}
	info, err := st.storage.Groups.GetGroup(group)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) && !errors.Is(err, storage.ErrStorageDisabled) {
			st.log.Errorf("Error getting group %v: %v", group, err)
		}
		return false
	}
	if info == nil {
		return false
	}
	for _, participant := range info.Participants {
		if !participant.IsAdmin && !participant.IsSuperAdmin {
			continue
		}
		for _, jid := range []types.JID{participant.JID, participant.LID, participant.PhoneNumber} {
			if st.sameJID(jid, user) {
				return true
			}
		}
	}
	return false
}
