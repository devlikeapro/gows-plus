package storage

import (
	"time"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type Storage struct {
	Messages             MessageStorage
	Contacts             ContactStorage
	Chats                ChatStorage
	Groups               GroupStorage
	ChatEphemeralSetting ChatEphemeralSettingStorage
	Labels               LabelStorage
	LabelAssociations    LabelAssociationStorage
	GroupMemberLabels    GroupMemberLabelStorage
	Lidmap               LidmapStorage
	CallLogs             CallLogStorage
}

type MessageStorage interface {
	UpsertOneMessage(msg *StoredMessage) error
	GetLastMessagesInChats(filter ChatFilter, sortBy Sort, pagination Pagination, merge bool) ([]*StoredMessage, error)
	GetAllMessages(filters MessageFilter, sortBy Sort, pagination Pagination, merge bool) ([]*StoredMessage, error)
	GetChatMessages(jid types.JID, filters MessageFilter, pagination Pagination, merge bool) ([]*StoredMessage, error)
	// GetMessage fetches a message by ID without retries.
	GetMessage(id types.MessageID) (*StoredMessage, error)
	// GetMessageWithRetries fetches a message by ID with retries.
	GetMessageWithRetries(id types.MessageID) (*StoredMessage, error)
	DeleteChatMessages(jid types.JID, deleteBefore time.Time) error
	DeleteMessage(id types.MessageID) error
}

type GroupStorage interface {
	FetchGroups(force bool) error
	UpdateGroup(update *events.GroupInfo) error
	UpsertOneGroup(group *types.GroupInfo) error
	GetAllGroups(sort Sort, pagination Pagination) ([]*types.GroupInfo, error)
	GetGroup(jid types.JID) (*types.GroupInfo, error)
	DeleteGroup(jid types.JID) error
	DeleteGroups() error
}

type ContactStorage interface {
	GetContact(user types.JID) (*StoredContact, error)
	GetAllContacts(sortBy Sort, pagination Pagination) ([]*StoredContact, error)
}

type ChatStorage interface {
	GetChats(filter ChatFilter, sortBy Sort, pagination Pagination, merge bool) ([]*StoredChat, error)
}

type ChatEphemeralSettingStorage interface {
	GetChatEphemeralSetting(id types.JID) (*StoredChatEphemeralSetting, error)
	UpdateChatEphemeralSetting(setting *StoredChatEphemeralSetting) error
	DeleteChatEphemeralSetting(id types.JID, deleteBefore time.Time) error
}

type LabelStorage interface {
	GetAllLabels() ([]*Label, error)
	GetLabelById(id string) (*Label, error)
	UpsertLabel(label *Label) error
	DeleteLabel(id string) error
}

type LabelAssociationStorage interface {
	GetJIDsByLabelID(labelID string) ([]types.JID, error)
	GetLabelIDsByJID(jid types.JID) ([]string, error)
	AddAssociation(jid types.JID, labelID string) error
	RemoveAssociation(jid types.JID, labelID string) error
	RemoveAssociationsByLabelID(labelID string) error
}

type GroupMemberLabelStorage interface {
	UpsertGroupMemberLabel(label *GroupMemberLabel) error
	UpsertGroupMemberLabels(labels []*GroupMemberLabel) error
	GetGroupMemberLabels(group types.JID) ([]*GroupMemberLabel, error)
	GetGroupMemberLabel(group types.JID, member types.JID) (*GroupMemberLabel, error)
	DeleteGroupMemberLabel(group types.JID, member types.JID) error
	DeleteGroupMemberLabels(group types.JID) error
}

type CallLogStorage interface {
	// UpsertCallLogs stores call log entries; an entry with a known call id replaces the stored one
	UpsertCallLogs(logs []*CallLog) error
	// GetCallLogs returns call log entries, newest first
	GetCallLogs(filter CallLogFilter, pagination Pagination) ([]*CallLog, error)
}

type LidmapStorage interface {
	// GetAllLidMap returns all lid/pn pairs from the database as an array of LidmapEntry
	GetAllLidMap() ([]LidmapEntry, error)
	// GetLidCount returns the count of lids in the database
	GetLidCount() (int, error)
}
