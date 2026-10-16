package gows

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestApplyTextEdit(t *testing.T) {
	quoted := &waE2E.ContextInfo{StanzaID: proto.String("QUOTED"), Participant: proto.String("100000000000002@lid")}
	cases := []struct {
		name     string
		original *waE2E.Message
		edited   *waE2E.Message
		applied  bool
		check    func(t *testing.T, m *waE2E.Message)
	}{
		{
			name:     "conversation",
			original: &waE2E.Message{Conversation: proto.String("old")},
			edited:   &waE2E.Message{Conversation: proto.String("new")},
			applied:  true,
			check: func(t *testing.T, m *waE2E.Message) {
				require.Equal(t, "new", m.GetConversation())
			},
		},
		{
			name:     "conversation edited as extended text",
			original: &waE2E.Message{Conversation: proto.String("old")},
			edited:   &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String("new")}},
			applied:  true,
			check: func(t *testing.T, m *waE2E.Message) {
				require.Equal(t, "new", m.GetConversation())
			},
		},
		{
			name:     "extended text keeps its context info",
			original: &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String("old"), ContextInfo: quoted}},
			edited:   &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String("new")}},
			applied:  true,
			check: func(t *testing.T, m *waE2E.Message) {
				require.Equal(t, "new", m.GetExtendedTextMessage().GetText())
				require.Equal(t, "QUOTED", m.GetExtendedTextMessage().GetContextInfo().GetStanzaID())
			},
		},
		{
			name:     "video caption",
			original: &waE2E.Message{VideoMessage: &waE2E.VideoMessage{Caption: proto.String("old"), DirectPath: proto.String("/v")}},
			edited:   &waE2E.Message{VideoMessage: &waE2E.VideoMessage{Caption: proto.String("new")}},
			applied:  true,
			check: func(t *testing.T, m *waE2E.Message) {
				require.Equal(t, "new", m.GetVideoMessage().GetCaption())
				require.Equal(t, "/v", m.GetVideoMessage().GetDirectPath())
			},
		},
		{
			name:     "caption removed",
			original: &waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: proto.String("old"), DirectPath: proto.String("/i")}},
			edited:   &waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: proto.String("")}},
			applied:  true,
			check: func(t *testing.T, m *waE2E.Message) {
				require.Equal(t, "", m.GetImageMessage().GetCaption())
				require.Equal(t, "/i", m.GetImageMessage().GetDirectPath())
			},
		},
		{
			name: "document with caption",
			original: &waE2E.Message{DocumentWithCaptionMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{
				DocumentMessage: &waE2E.DocumentMessage{Caption: proto.String("old"), FileName: proto.String("report.pdf")},
			}}},
			edited: &waE2E.Message{DocumentWithCaptionMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{
				DocumentMessage: &waE2E.DocumentMessage{Caption: proto.String("new")},
			}}},
			applied: true,
			check: func(t *testing.T, m *waE2E.Message) {
				doc := m.GetDocumentWithCaptionMessage().GetMessage().GetDocumentMessage()
				require.Equal(t, "new", doc.GetCaption())
				require.Equal(t, "report.pdf", doc.GetFileName())
			},
		},
		{
			name:     "edit inside an ephemeral wrapper",
			original: &waE2E.Message{Conversation: proto.String("old")},
			edited:   &waE2E.Message{EphemeralMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{Conversation: proto.String("new")}}},
			applied:  true,
			check: func(t *testing.T, m *waE2E.Message) {
				require.Equal(t, "new", m.GetConversation())
			},
		},
		{
			name:     "no text to replace",
			original: &waE2E.Message{StickerMessage: &waE2E.StickerMessage{DirectPath: proto.String("/s")}},
			edited:   &waE2E.Message{Conversation: proto.String("new")},
			applied:  false,
			check: func(t *testing.T, m *waE2E.Message) {
				require.Equal(t, "/s", m.GetStickerMessage().GetDirectPath())
			},
		},
		{
			name:     "edit without text",
			original: &waE2E.Message{Conversation: proto.String("old")},
			edited:   &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{Text: proto.String("👍")}},
			applied:  false,
			check: func(t *testing.T, m *waE2E.Message) {
				require.Equal(t, "old", m.GetConversation())
			},
		},
		{
			name:     "nil edit",
			original: &waE2E.Message{Conversation: proto.String("old")},
			edited:   nil,
			applied:  false,
			check: func(t *testing.T, m *waE2E.Message) {
				require.Equal(t, "old", m.GetConversation())
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.applied, applyTextEdit(tc.original, tc.edited))
			tc.check(t, tc.original)
		})
	}
	require.False(t, applyTextEdit(nil, &waE2E.Message{Conversation: proto.String("new")}))
}
