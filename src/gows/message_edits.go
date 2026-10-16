package gows

import (
	"context"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func (gows *GoWS) handleSecretMessageEdit(ctx context.Context, msg *events.Message) {
	sem := msg.Message.GetSecretEncryptedMessage()
	decryptedMsg, err := gows.DecryptSecretEncryptedMessage(ctx, msg)
	if err != nil {
		gows.Log.Warnf("Failed to decrypt secret message edit %s: %v", msg.Info.ID, err)
		gows.emitEvent(msg)
		return
	}
	msg.Message = &waE2E.Message{
		ProtocolMessage: &waE2E.ProtocolMessage{
			Key:           sem.GetTargetMessageKey(),
			Type:          waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
			EditedMessage: decryptedMsg,
		},
	}
	gows.emitEvent(msg)
}

// applyTextEdit replaces the text or the caption of a stored message with the one of an edit,
// keeping everything else (media, context info), and reports whether it had one to replace
func applyTextEdit(original, edited *waE2E.Message) bool {
	text, ok := editText(edited)
	if !ok {
		return false
	}
	target := unwrapTextContent(original)
	switch {
	case target == nil:
		return false
	case target.Conversation != nil:
		target.Conversation = proto.String(text)
	case target.ExtendedTextMessage != nil:
		target.ExtendedTextMessage.Text = proto.String(text)
	case target.ImageMessage != nil:
		target.ImageMessage.Caption = proto.String(text)
	case target.VideoMessage != nil:
		target.VideoMessage.Caption = proto.String(text)
	case target.DocumentMessage != nil:
		target.DocumentMessage.Caption = proto.String(text)
	default:
		return false
	}
	return true
}

// editText returns the text carried by the content of an edit.
func editText(edited *waE2E.Message) (string, bool) {
	content := unwrapTextContent(edited)
	switch {
	case content == nil:
		return "", false
	case content.Conversation != nil:
		return content.GetConversation(), true
	case content.ExtendedTextMessage != nil:
		return content.GetExtendedTextMessage().GetText(), true
	case content.ImageMessage != nil:
		return content.GetImageMessage().GetCaption(), true
	case content.VideoMessage != nil:
		return content.GetVideoMessage().GetCaption(), true
	case content.DocumentMessage != nil:
		return content.GetDocumentMessage().GetCaption(), true
	default:
		return "", false
	}
}

// unwrapTextContent returns the message that holds the text, past its wrappers
func unwrapTextContent(msg *waE2E.Message) *waE2E.Message {
	for msg != nil {
		switch {
		case msg.GetEphemeralMessage().GetMessage() != nil:
			msg = msg.GetEphemeralMessage().GetMessage()
		case msg.GetDocumentWithCaptionMessage().GetMessage() != nil:
			msg = msg.GetDocumentWithCaptionMessage().GetMessage()
		case msg.GetEditedMessage().GetMessage() != nil:
			msg = msg.GetEditedMessage().GetMessage()
		default:
			return msg
		}
	}
	return nil
}
