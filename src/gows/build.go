package gows

import (
	"errors"
	"time"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

var (
	// Overridable via WAHA_GOWS_LINK_PREVIEW_TIMEOUT (set at startup in main).
	FetchPreviewTimeout = 10 * time.Second
)

func (gows *GoWS) BuildConversationMessage(text string) *waE2E.Message {
	message := waE2E.Message{}
	message.Conversation = proto.String(text)
	return &message
}

// BuildExtendedTextMessage builds a text message and adds a link preview if requested.
func (gows *GoWS) BuildExtendedTextMessage(text string) *waE2E.Message {
	message := waE2E.Message{}
	message.ExtendedTextMessage = &waE2E.ExtendedTextMessage{
		Text: proto.String(text),
	}
	return &message
}

// BuildEditedMessage builds a new message for editing an existing message,
// preserving the original message's type and context info when possible.
func (gows *GoWS) BuildEditedMessage(
	jid types.JID,
	text string,
	originalMessage *waE2E.Message,
) *waE2E.Message {
	switch {
	case originalMessage != nil && originalMessage.GetConversation() != "":
		// Keep the current behavior for plain conversation messages.
		return gows.BuildConversationMessage(text)
	case originalMessage != nil && originalMessage.GetImageMessage() != nil:
		return &waE2E.Message{
			ImageMessage: &waE2E.ImageMessage{
				Caption:     proto.String(text),
				ContextInfo: originalMessage.GetImageMessage().GetContextInfo(),
			},
		}
	case originalMessage != nil && originalMessage.GetVideoMessage() != nil:
		return &waE2E.Message{
			VideoMessage: &waE2E.VideoMessage{
				Caption:     proto.String(text),
				ContextInfo: originalMessage.GetVideoMessage().GetContextInfo(),
			},
		}
	case originalMessage != nil && originalMessage.GetDocumentMessage() != nil:
		return &waE2E.Message{
			DocumentMessage: &waE2E.DocumentMessage{
				Caption:     proto.String(text),
				ContextInfo: originalMessage.GetDocumentMessage().GetContextInfo(),
			},
		}
	case originalMessage != nil &&
		originalMessage.GetDocumentWithCaptionMessage() != nil &&
		originalMessage.GetDocumentWithCaptionMessage().GetMessage() != nil &&
		originalMessage.GetDocumentWithCaptionMessage().GetMessage().GetDocumentMessage() != nil:
		return &waE2E.Message{
			DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
				Message: &waE2E.Message{
					DocumentMessage: &waE2E.DocumentMessage{
						Caption: proto.String(text),
						ContextInfo: originalMessage.GetDocumentWithCaptionMessage().
							GetMessage().
							GetDocumentMessage().
							GetContextInfo(),
					},
				},
			},
		}
	default:
		var contextInfo = ExtractContextInfo(originalMessage)
		message := &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text:        proto.String(text),
				ContextInfo: contextInfo,
			},
		}
		return message
	}
}

// BuildEdit builds a message edit message using the given variables.
// The built message can be sent normally using Client.SendMessage.
//
// Adjusted from the original meow BuildEdit - it counts for participants (groups)
//
//	resp, err := cli.SendMessage(context.Background(), chat, cli.BuildEdit(chat, originalMessageID, &waE2E.Message{
//		Conversation: proto.String("edited message"),
//	})
func (gows *GoWS) BuildEdit(chat types.JID, id types.MessageID, newContent *waE2E.Message) *waE2E.Message {
	key := &waCommon.MessageKey{
		FromMe:    proto.Bool(true),
		ID:        proto.String(id),
		RemoteJID: proto.String(chat.String()),
	}
	// If the chat is a group, set the participant
	if chat.Server == types.GroupServer {
		key.Participant = proto.String(gows.int.GetOwnID().ToNonAD().String())
	}
	protocol := &waE2E.ProtocolMessage{
		Key:           key,
		Type:          waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
		EditedMessage: newContent,
		TimestampMS:   proto.Int64(time.Now().UnixMilli()),
	}

	// Keep newsletter edit encoding compatible with whatsmeow's newsletter sender path.
	if chat.Server == types.NewsletterServer {
		return &waE2E.Message{
			EditedMessage: &waE2E.FutureProofMessage{
				Message: &waE2E.Message{
					ProtocolMessage: protocol,
				},
			},
		}
	}

	return &waE2E.Message{
		ProtocolMessage: protocol,
	}
}

func (gows *GoWS) PopulateContextInfoWithReply(info *waE2E.ContextInfo, replyToId types.MessageID) (*waE2E.ContextInfo, error) {
	msg, err := gows.Storage.Messages.GetMessageWithRetries(replyToId)
	if err != nil {
		return info, err
	}

	if info == nil {
		info = &waE2E.ContextInfo{}
	}

	quoted := msg.Message.Message
	quoted.MessageContextInfo = nil
	info.StanzaID = proto.String(msg.Info.ID)
	info.Participant = proto.String(msg.Info.Sender.ToNonAD().String())
	info.QuotedMessage = quoted
	return info, nil
}

func (gows *GoWS) PopulateContextInfoWithMentions(info *waE2E.ContextInfo, mentions []string) *waE2E.ContextInfo {
	if len(mentions) == 0 {
		return info
	}
	if info == nil {
		info = &waE2E.ContextInfo{}
	}
	info.MentionedJID = mentions
	return info
}

// ExtractContextInfo returns the ContextInfo of the message content, nil for plain text
func ExtractContextInfo(msg *waE2E.Message) *waE2E.ContextInfo {
	if msg == nil {
		return nil
	}
	switch {
	case msg.Conversation != nil:
		return nil
	case msg.ExtendedTextMessage != nil:
		return msg.ExtendedTextMessage.ContextInfo
	case msg.ImageMessage != nil:
		return msg.ImageMessage.ContextInfo
	case msg.ContactMessage != nil:
		return msg.ContactMessage.ContextInfo
	case msg.LocationMessage != nil:
		return msg.LocationMessage.ContextInfo
	case msg.VideoMessage != nil:
		return msg.VideoMessage.ContextInfo
	case msg.PtvMessage != nil:
		return msg.PtvMessage.ContextInfo
	case msg.AudioMessage != nil:
		return msg.AudioMessage.ContextInfo
	case msg.DocumentMessage != nil:
		return msg.DocumentMessage.ContextInfo
	case msg.DocumentWithCaptionMessage != nil && msg.DocumentWithCaptionMessage.Message != nil && msg.DocumentWithCaptionMessage.Message.DocumentMessage != nil:
		return msg.DocumentWithCaptionMessage.Message.DocumentMessage.ContextInfo
	case msg.StickerMessage != nil:
		return msg.StickerMessage.ContextInfo
	case msg.ContactsArrayMessage != nil:
		return msg.ContactsArrayMessage.ContextInfo
	case msg.TemplateMessage != nil:
		return msg.TemplateMessage.ContextInfo
	case msg.ListMessage != nil:
		return msg.ListMessage.ContextInfo
	case msg.PollCreationMessage != nil:
		return msg.PollCreationMessage.ContextInfo
	case msg.PollCreationMessageV2 != nil:
		return msg.PollCreationMessageV2.ContextInfo
	case msg.PollCreationMessageV3 != nil:
		return msg.PollCreationMessageV3.ContextInfo
	default:
		return nil
	}
}

// SetContextInfo attaches info to the message content, false if it can't carry one
func SetContextInfo(msg *waE2E.Message, info *waE2E.ContextInfo) bool {
	if msg == nil {
		return false
	}
	switch {
	case msg.ExtendedTextMessage != nil:
		msg.ExtendedTextMessage.ContextInfo = info
	case msg.ImageMessage != nil:
		msg.ImageMessage.ContextInfo = info
	case msg.ContactMessage != nil:
		msg.ContactMessage.ContextInfo = info
	case msg.LocationMessage != nil:
		msg.LocationMessage.ContextInfo = info
	case msg.VideoMessage != nil:
		msg.VideoMessage.ContextInfo = info
	case msg.PtvMessage != nil:
		msg.PtvMessage.ContextInfo = info
	case msg.AudioMessage != nil:
		msg.AudioMessage.ContextInfo = info
	case msg.DocumentMessage != nil:
		msg.DocumentMessage.ContextInfo = info
	case msg.DocumentWithCaptionMessage != nil && msg.DocumentWithCaptionMessage.Message != nil && msg.DocumentWithCaptionMessage.Message.DocumentMessage != nil:
		msg.DocumentWithCaptionMessage.Message.DocumentMessage.ContextInfo = info
	case msg.StickerMessage != nil:
		msg.StickerMessage.ContextInfo = info
	case msg.ContactsArrayMessage != nil:
		msg.ContactsArrayMessage.ContextInfo = info
	case msg.TemplateMessage != nil:
		msg.TemplateMessage.ContextInfo = info
	case msg.ListMessage != nil:
		msg.ListMessage.ContextInfo = info
	case msg.PollCreationMessage != nil:
		msg.PollCreationMessage.ContextInfo = info
	case msg.PollCreationMessageV2 != nil:
		msg.PollCreationMessageV2.ContextInfo = info
	case msg.PollCreationMessageV3 != nil:
		msg.PollCreationMessageV3.ContextInfo = info
	default:
		return false
	}
	return true
}

var ErrCannotForward = errors.New("this message type cannot be forwarded")

var ErrCannotForwardPoll = errors.New("a poll cannot be forwarded")

func isPollCreation(msg *waE2E.Message) bool {
	return msg.GetPollCreationMessage() != nil ||
		msg.GetPollCreationMessageV2() != nil ||
		msg.GetPollCreationMessageV3() != nil
}

// BuildForwardedMessage builds a forwarded copy of original (media keys reused, not re-uploaded).
// base is mutated and becomes the ContextInfo of the result; the original's quote and mentions are dropped.
func BuildForwardedMessage(original *events.Message, base *waE2E.ContextInfo, force bool) (*waE2E.Message, error) {
	if original == nil || original.Message == nil {
		return nil, ErrCannotForward
	}
	// Polls can't be forwarded - the votes' secret belongs to the original
	if isPollCreation(original.Message) {
		return nil, ErrCannotForwardPoll
	}
	// original.Message is already unwrapped by whatsmeow (view-once, ephemeral, etc)
	content := proto.Clone(original.Message).(*waE2E.Message)
	// Device list and message secret belong to the original delivery
	content.MessageContextInfo = nil

	// Plain text has no ContextInfo - promote it to ExtendedTextMessage
	if content.Conversation != nil {
		content.ExtendedTextMessage = &waE2E.ExtendedTextMessage{
			Text: proto.String(content.GetConversation()),
		}
		content.Conversation = nil
	}

	score := ExtractContextInfo(content).GetForwardingScore()
	// Own messages are not marked as forwarded (like official clients) unless forced
	if !original.Info.IsFromMe || force {
		score++
	}

	info := base
	if info == nil {
		info = &waE2E.ContextInfo{}
	}
	if score > 0 {
		info.IsForwarded = proto.Bool(true)
		info.ForwardingScore = proto.Uint32(score)
	}
	if !SetContextInfo(content, info) {
		return nil, ErrCannotForward
	}
	return content, nil
}

type Contact struct {
	DisplayName string
	Vcard       string
}

func buildContactMessage(contact Contact) *waE2E.ContactMessage {
	return &waE2E.ContactMessage{
		DisplayName: proto.String(contact.DisplayName),
		Vcard:       proto.String(contact.Vcard),
	}
}

func BuildContactsMessage(contacts []Contact, contextInfo *waE2E.ContextInfo) (message *waE2E.Message) {
	if len(contacts) == 0 {
		return nil
	}

	// Single contact
	if len(contacts) == 1 {
		message = &waE2E.Message{
			ContactMessage: buildContactMessage(contacts[0]),
		}
		message.ContactMessage.ContextInfo = contextInfo
		return message
	}
	// Multiple contacts
	message = &waE2E.Message{
		ContactsArrayMessage: &waE2E.ContactsArrayMessage{
			Contacts: make([]*waE2E.ContactMessage, len(contacts)),
		},
	}
	for i, contact := range contacts {
		message.ContactsArrayMessage.Contacts[i] = buildContactMessage(contact)
	}
	message.ContactsArrayMessage.ContextInfo = contextInfo
	return message
}

func BuildContactUpdate(jid types.JID, firstName, lastName string) appstate.PatchInfo {
	fullName := firstName
	if lastName != "" {
		fullName = firstName + " " + lastName
	}

	return appstate.PatchInfo{
		Type: appstate.WAPatchCriticalUnblockLow,
		Mutations: []appstate.MutationInfo{{
			Index:   []string{appstate.IndexContact, jid.String()},
			Version: 2,
			Value: &waSyncAction.SyncActionValue{
				ContactAction: &waSyncAction.ContactAction{
					FullName:                 proto.String(fullName),
					FirstName:                proto.String(firstName),
					SaveOnPrimaryAddressbook: proto.Bool(true),
				},
			},
		}},
	}
}

// BuildChatUnread builds an app state patch for marking a chat as read or unread.
//
// The lastMessageKeys parameter should contain the message keys to mark as read.
// If markRead is true, the chat will be marked as read, otherwise it will be marked as unread.
// Note: Only MessageKey is accepted; timestamps are not required.
func BuildChatUnread(
	jid types.JID,
	markRead bool,
	lastMessageKeys []*waCommon.MessageKey,
	lastMessageTimestamp time.Time,
) appstate.PatchInfo {
	messageRange := &waSyncAction.SyncActionMessageRange{
		LastMessageTimestamp: proto.Int64(lastMessageTimestamp.UnixMilli()),
	}

	if len(lastMessageKeys) > 0 {
		// Convert keys to SyncActionMessage without timestamps, as only keys are required
		msgs := make([]*waSyncAction.SyncActionMessage, len(lastMessageKeys))
		for i, key := range lastMessageKeys {
			msgs[i] = &waSyncAction.SyncActionMessage{Key: key}
		}
		messageRange.Messages = msgs
	}

	return appstate.PatchInfo{
		Type: appstate.WAPatchRegularLow,
		Mutations: []appstate.MutationInfo{{
			Index:   []string{appstate.IndexMarkChatAsRead, jid.String()},
			Version: 3,
			Value: &waSyncAction.SyncActionValue{
				MarkChatAsReadAction: &waSyncAction.MarkChatAsReadAction{
					Read:         proto.Bool(markRead),
					MessageRange: messageRange,
				},
			},
		}},
	}
}
