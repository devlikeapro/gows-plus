package noop

import (
	"time"

	"github.com/devlikeapro/gows/storage"
	"go.mau.fi/whatsmeow/types"
)

type MessageStorage struct{}

var _ storage.MessageStorage = (*MessageStorage)(nil)

func NewMessageStorage() *MessageStorage {
	return &MessageStorage{}
}

func (s MessageStorage) UpsertOneMessage(msg *storage.StoredMessage) error {
	return nil
}

func (s MessageStorage) GetLastMessagesInChats(filter storage.ChatFilter, sortBy storage.Sort, pagination storage.Pagination, merge bool) ([]*storage.StoredMessage, error) {
	return []*storage.StoredMessage{}, nil
}

func (s MessageStorage) GetAllMessages(filters storage.MessageFilter, sortBy storage.Sort, pagination storage.Pagination, merge bool) ([]*storage.StoredMessage, error) {
	return []*storage.StoredMessage{}, nil
}

func (s MessageStorage) GetChatMessages(jid types.JID, filters storage.MessageFilter, pagination storage.Pagination, merge bool) ([]*storage.StoredMessage, error) {
	return []*storage.StoredMessage{}, nil
}

func (s MessageStorage) GetMessage(id types.MessageID) (*storage.StoredMessage, error) {
	return nil, storage.StorageDisabled("messages")
}

func (s MessageStorage) GetMessageWithRetries(id types.MessageID) (*storage.StoredMessage, error) {
	return nil, storage.StorageDisabled("messages")
}

func (s MessageStorage) DeleteChatMessages(jid types.JID, deleteBefore time.Time) error {
	return nil
}

func (s MessageStorage) DeleteMessage(id types.MessageID) error {
	return nil
}

func (s MessageStorage) UpdateMessage(id types.MessageID, update func(msg *storage.StoredMessage) (bool, error)) error {
	return storage.StorageDisabled("messages")
}
