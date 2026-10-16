package gows

import (
	"context"
	"errors"

	"github.com/devlikeapro/gows/storage"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// BuildDeleteMessagesForMe builds a "Delete for me" app state patch.
// Duplicates are skipped: the same index twice in one patch breaks the patch hash on other devices
func BuildDeleteMessagesForMe(messages []*types.MessageInfo, deleteMedia bool) appstate.PatchInfo {
	type messageKey struct {
		chat types.JID
		id   types.MessageID
	}
	seen := make(map[messageKey]bool, len(messages))
	mutations := make([]appstate.MutationInfo, 0, len(messages))
	for _, info := range messages {
		chat := info.Chat.ToNonAD()
		key := messageKey{chat: chat, id: info.ID}
		if seen[key] {
			continue
		}
		seen[key] = true

		fromMe := "0"
		if info.IsFromMe {
			fromMe = "1"
		}
		// Sender only for messages others sent in a group
		participant := "0"
		if chat.Server == types.GroupServer && !info.IsFromMe && !info.Sender.IsEmpty() {
			participant = info.Sender.ToNonAD().String()
		}
		mutations = append(mutations, appstate.MutationInfo{
			Index:   []string{appstate.IndexDeleteMessageForMe, chat.String(), info.ID, fromMe, participant},
			Version: 3,
			Value: &waSyncAction.SyncActionValue{
				DeleteMessageForMeAction: &waSyncAction.DeleteMessageForMeAction{
					DeleteMedia:      proto.Bool(deleteMedia),
					MessageTimestamp: proto.Int64(info.Timestamp.Unix()),
				},
			},
		})
	}
	return appstate.PatchInfo{
		Type:      appstate.WAPatchRegularHigh,
		Mutations: mutations,
	}
}

// IsSameChat tells if a and b are the same chat, LID and phone number of a direct chat included
func (gows *GoWS) IsSameChat(ctx context.Context, a, b types.JID) (bool, error) {
	if gows.Client == nil || gows.Store == nil || gows.Store.LIDs == nil {
		return isSameChat(a, b, nil, nil)
	}
	pnForLID := func(lid types.JID) (types.JID, error) {
		return gows.Store.LIDs.GetPNForLID(ctx, lid)
	}
	return isSameChat(a, b, pnForLID, gows.validLIDPNPair)
}

func isSameChat(a, b types.JID, pnForLID func(lid types.JID) (types.JID, error), valid func(lid, pn types.JID) bool) (bool, error) {
	a, b = a.ToNonAD(), b.ToNonAD()
	if a == b {
		return true, nil
	}
	var lid, pn types.JID
	switch {
	case a.Server == types.HiddenUserServer && b.Server == types.DefaultUserServer:
		lid, pn = a, b
	case a.Server == types.DefaultUserServer && b.Server == types.HiddenUserServer:
		lid, pn = b, a
	default:
		return false, nil
	}
	if pnForLID == nil {
		return false, nil
	}
	stored, err := pnForLID(lid)
	if err != nil {
		return false, err
	}
	if stored.IsEmpty() || stored.User != pn.User {
		return false, nil
	}
	return valid == nil || valid(lid, pn), nil
}

// handleDeleteForMe removes the message from storage, unless it is stored in another chat
func (st *StorageEventHandler) handleDeleteForMe(event *events.DeleteForMe) {
	stored, err := st.storage.Messages.GetMessage(event.MessageID)
	if errors.Is(err, storage.ErrNotFound) || errors.Is(err, storage.ErrStorageDisabled) {
		return
	}
	if err != nil {
		st.log.Errorf("Failed to get message %v deleted for me: %v", event.MessageID, err)
		return
	}
	if stored == nil || stored.Message == nil {
		return
	}
	same, err := st.isSameChat(stored.Info.Chat, event.ChatJID)
	if err != nil {
		st.log.Errorf("Failed to check the chat of message %v deleted for me: %v", event.MessageID, err)
		return
	}
	if !same {
		st.log.Warnf("Keeping message %v: it was deleted for me in %v, but it is stored in %v", event.MessageID, event.ChatJID, stored.Info.Chat)
		return
	}
	if err = st.storage.Messages.DeleteMessage(event.MessageID); err != nil {
		st.log.Errorf("Failed to delete message %v deleted for me: %v", event.MessageID, err)
	}
}

func (st *StorageEventHandler) isSameChat(a, b types.JID) (bool, error) {
	if st.gows == nil {
		return isSameChat(a, b, nil, nil)
	}
	return st.gows.IsSameChat(st.gows.Context, a, b)
}
