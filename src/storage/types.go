package storage

import (
	"errors"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type Status int

const (
	StatusError       Status = 0
	StatusPending     Status = 1
	StatusServerAck   Status = 2
	StatusDeliveryAck Status = 3
	StatusRead        Status = 4
	StatusPlayed      Status = 5
)

// StoredMessage contains a message and some additional data.
type StoredMessage struct {
	*events.Message
	IsReal bool
	Status *Status
}

type StoredContact struct {
	Jid      types.JID
	Name     string
	PushName string
}

type StoredChat struct {
	Jid                   types.JID
	Name                  string
	ConversationTimestamp time.Time
}

type EphemeralSetting struct {
	Initiator     *waE2E.DisappearingMode_Initiator
	Trigger       *waE2E.DisappearingMode_Trigger
	InitiatedByMe *bool
	Timestamp     *int64
	Expiration    uint32
}

type StoredChatEphemeralSetting struct {
	ID          types.JID
	IsEphemeral bool
	Setting     *EphemeralSetting
}

func NotEphemeral(jid types.JID) *StoredChatEphemeralSetting {
	return &StoredChatEphemeralSetting{
		ID:          jid,
		IsEphemeral: false,
	}
}

type SortOrder string

const (
	SortAsc  SortOrder = "ASC"
	SortDesc SortOrder = "DESC"
)

type Sort struct {
	Field string
	Order SortOrder
}

type Pagination struct {
	Offset uint64
	Limit  uint64
}

type MessageFilter struct {
	Jid          *types.JID
	TimestampGte *time.Time
	TimestampLte *time.Time
	FromMe       *bool
	Status       *Status
}

type ChatFilter struct {
	Jids []types.JID
}

// Label represents a label that can be associated with chats
type Label struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Color    int    `json:"color"`
	Type     *int   `json:"type,omitempty"`
	IsActive *bool  `json:"isActive,omitempty"`
}

// LabelAssociation represents an association between a label and a JID
type LabelAssociation struct {
	JID     types.JID `json:"jid"`
	LabelID string    `json:"label_id"`
}

// LidmapEntry represents a lid/pn pair
type LidmapEntry struct {
	Lid types.JID `json:"lid"`
	Pn  types.JID `json:"pn"`
}

// GroupMemberLabel is a group member tag, keyed by the member JID as received
type GroupMemberLabel struct {
	GroupJID  types.JID `json:"groupJid"`
	MemberJID types.JID `json:"memberJid"` // LID in LID-addressed groups, phone number otherwise
	Label     string    `json:"label"`     // empty when the tag was removed
	Timestamp time.Time `json:"timestamp"`
}

var ErrEmptyGroupMember = errors.New("group member label without a member")

// CallLog is one entry of the account's call log - what the phone shows in the chat as a call bubble
type CallLog struct {
	ID        string                      `json:"id"`
	Chat      types.JID                   `json:"chat"`      // the other party, or the group of a group call; empty when unknown
	Timestamp time.Time                   `json:"timestamp"` // call start
	FromMe    bool                        `json:"fromMe"`    // this account placed the call
	Record    *waSyncAction.CallLogRecord `json:"record"`    // as WhatsApp sent it: result, duration, video, participants
}

type CallLogFilter struct {
	Jid *types.JID
}

var ErrEmptyCallLog = errors.New("call log without a call id")
