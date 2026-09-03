package gows

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func incoming(msg *waE2E.Message) *events.Message {
	return &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{IsFromMe: false},
		},
		Message: msg,
	}
}

func outgoing(msg *waE2E.Message) *events.Message {
	return &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{IsFromMe: true},
		},
		Message: msg,
	}
}

// Plain text has no ContextInfo field, so forwarding one has to promote it to an
// extended text message. Without this the message goes out with no "forwarded"
// marker at all, and nothing reports a problem.
func TestBuildForwardedMessage_PromotesPlainTextToExtended(t *testing.T) {
	original := incoming(&waE2E.Message{Conversation: proto.String("hello")})

	out, err := BuildForwardedMessage(original, nil, false)

	require.NoError(t, err)
	assert.Nil(t, out.Conversation, "the plain text field must be cleared")
	require.NotNil(t, out.ExtendedTextMessage)
	assert.Equal(t, "hello", out.ExtendedTextMessage.GetText())
	assert.True(t, out.ExtendedTextMessage.ContextInfo.GetIsForwarded())
	assert.EqualValues(t, 1, out.ExtendedTextMessage.ContextInfo.GetForwardingScore())
}

// Media is forwarded by re-sending the original keys, not by uploading it
// again - that is what makes a forward cheap and what the official clients do.
func TestBuildForwardedMessage_KeepsMediaKeys(t *testing.T) {
	original := incoming(&waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			URL:           proto.String("https://mmg.whatsapp.net/x"),
			DirectPath:    proto.String("/v/t62.7118-24/abc"),
			MediaKey:      []byte("media-key"),
			FileEncSHA256: []byte("enc-sha"),
			FileSHA256:    []byte("sha"),
			Mimetype:      proto.String("image/jpeg"),
			Caption:       proto.String("look"),
		},
	})

	out, err := BuildForwardedMessage(original, nil, false)

	require.NoError(t, err)
	require.NotNil(t, out.ImageMessage)
	assert.Equal(t, "/v/t62.7118-24/abc", out.ImageMessage.GetDirectPath())
	assert.Equal(t, []byte("media-key"), out.ImageMessage.GetMediaKey())
	assert.Equal(t, []byte("enc-sha"), out.ImageMessage.GetFileEncSHA256())
	assert.Equal(t, "look", out.ImageMessage.GetCaption())
	assert.True(t, out.ImageMessage.ContextInfo.GetIsForwarded())
}

// The score travels with the message, so a chain of forwards keeps counting.
// Clients show "forwarded many times" from 5 onwards.
func TestBuildForwardedMessage_IncrementsExistingScore(t *testing.T) {
	original := incoming(&waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String("chain"),
			ContextInfo: &waE2E.ContextInfo{
				IsForwarded:     proto.Bool(true),
				ForwardingScore: proto.Uint32(4),
			},
		},
	})

	out, err := BuildForwardedMessage(original, nil, false)

	require.NoError(t, err)
	assert.EqualValues(t, 5, out.ExtendedTextMessage.ContextInfo.GetForwardingScore())
}

// Forwarding something you sent yourself is not marked as forwarded, matching
// the official clients.
func TestBuildForwardedMessage_OwnMessageIsNotMarked(t *testing.T) {
	original := outgoing(&waE2E.Message{Conversation: proto.String("mine")})

	out, err := BuildForwardedMessage(original, nil, false)

	require.NoError(t, err)
	assert.False(t, out.ExtendedTextMessage.ContextInfo.GetIsForwarded())
	assert.EqualValues(t, 0, out.ExtendedTextMessage.ContextInfo.GetForwardingScore())
}

// ...unless the caller asks for it.
func TestBuildForwardedMessage_OwnMessageWithForce(t *testing.T) {
	original := outgoing(&waE2E.Message{Conversation: proto.String("mine")})

	out, err := BuildForwardedMessage(original, nil, true)

	require.NoError(t, err)
	assert.True(t, out.ExtendedTextMessage.ContextInfo.GetIsForwarded())
	assert.EqualValues(t, 1, out.ExtendedTextMessage.ContextInfo.GetForwardingScore())
}

// The disappearing-message settings worked out for the destination chat have to
// survive - a forward into an ephemeral chat must still expire.
func TestBuildForwardedMessage_KeepsDestinationContext(t *testing.T) {
	original := incoming(&waE2E.Message{Conversation: proto.String("hi")})
	base := &waE2E.ContextInfo{Expiration: proto.Uint32(604800)}

	out, err := BuildForwardedMessage(original, base, false)

	require.NoError(t, err)
	ci := out.ExtendedTextMessage.ContextInfo
	assert.EqualValues(t, 604800, ci.GetExpiration())
	assert.True(t, ci.GetIsForwarded())
}

// A forward carries the content, not the conversation it came from: whatever the
// original quoted or mentioned belongs to the other chat.
func TestBuildForwardedMessage_DropsOriginalQuoteAndMentions(t *testing.T) {
	original := incoming(&waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String("reply of mine"),
			ContextInfo: &waE2E.ContextInfo{
				StanzaID:      proto.String("QUOTED-ID"),
				Participant:   proto.String("111@s.whatsapp.net"),
				QuotedMessage: &waE2E.Message{Conversation: proto.String("quoted")},
				MentionedJID:  []string{"222@s.whatsapp.net"},
			},
		},
	})

	out, err := BuildForwardedMessage(original, nil, false)

	require.NoError(t, err)
	ci := out.ExtendedTextMessage.ContextInfo
	assert.Empty(t, ci.GetStanzaID())
	assert.Nil(t, ci.GetQuotedMessage())
	assert.Empty(t, ci.GetMentionedJID())
	assert.True(t, ci.GetIsForwarded())
}

// Content with nowhere to carry the markers must be refused, not sent as a
// forward that is not marked as one.
func TestBuildForwardedMessage_RefusesUnsupportedContent(t *testing.T) {
	original := incoming(&waE2E.Message{
		ReactionMessage: &waE2E.ReactionMessage{Text: proto.String("👍")},
	})

	out, err := BuildForwardedMessage(original, nil, false)

	assert.ErrorIs(t, err, ErrCannotForward)
	assert.Nil(t, out)
}

func TestBuildForwardedMessage_RefusesEmptyMessage(t *testing.T) {
	out, err := BuildForwardedMessage(nil, nil, false)
	assert.ErrorIs(t, err, ErrCannotForward)
	assert.Nil(t, out)

	out, err = BuildForwardedMessage(&events.Message{}, nil, false)
	assert.ErrorIs(t, err, ErrCannotForward)
	assert.Nil(t, out)
}

// The message handed over comes from the store, which caches it - mutating it
// would corrupt the stored copy for every later read.
func TestBuildForwardedMessage_DoesNotMutateTheOriginal(t *testing.T) {
	original := incoming(&waE2E.Message{
		Conversation:       proto.String("untouched"),
		MessageContextInfo: &waE2E.MessageContextInfo{MessageSecret: []byte("secret")},
	})

	_, err := BuildForwardedMessage(original, nil, false)

	require.NoError(t, err)
	assert.Equal(t, "untouched", original.Message.GetConversation())
	assert.Nil(t, original.Message.ExtendedTextMessage)
	assert.NotNil(t, original.Message.MessageContextInfo, "the stored copy keeps its own secrets")
}

// The message secret belongs to the delivery it came from; whatsmeow puts a new
// one on the message we are about to send.
func TestBuildForwardedMessage_DropsMessageContextInfo(t *testing.T) {
	original := incoming(&waE2E.Message{
		ImageMessage:       &waE2E.ImageMessage{MediaKey: []byte("k")},
		MessageContextInfo: &waE2E.MessageContextInfo{MessageSecret: []byte("secret")},
	})

	out, err := BuildForwardedMessage(original, nil, false)

	require.NoError(t, err)
	assert.Nil(t, out.MessageContextInfo)
}

// SetContextInfo is the writing half of ContextInfoOf: whatever one can read,
// the other has to be able to write, or a forward of that type loses its marker.
func TestSetContextInfo_CoversWhatContextInfoOfReads(t *testing.T) {
	samples := map[string]*waE2E.Message{
		"extendedText":  {ExtendedTextMessage: &waE2E.ExtendedTextMessage{}},
		"image":         {ImageMessage: &waE2E.ImageMessage{}},
		"contact":       {ContactMessage: &waE2E.ContactMessage{}},
		"location":      {LocationMessage: &waE2E.LocationMessage{}},
		"video":         {VideoMessage: &waE2E.VideoMessage{}},
		"ptv":           {PtvMessage: &waE2E.VideoMessage{}},
		"audio":         {AudioMessage: &waE2E.AudioMessage{}},
		"document":      {DocumentMessage: &waE2E.DocumentMessage{}},
		"sticker":       {StickerMessage: &waE2E.StickerMessage{}},
		"contactsArray": {ContactsArrayMessage: &waE2E.ContactsArrayMessage{}},
		"template":      {TemplateMessage: &waE2E.TemplateMessage{}},
		"list":          {ListMessage: &waE2E.ListMessage{}},
		"poll":          {PollCreationMessage: &waE2E.PollCreationMessage{}},
		"pollV2":        {PollCreationMessageV2: &waE2E.PollCreationMessage{}},
		"pollV3":        {PollCreationMessageV3: &waE2E.PollCreationMessage{}},
		"documentWithCaption": {DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{}},
		}},
	}

	info := &waE2E.ContextInfo{IsForwarded: proto.Bool(true)}
	for name, msg := range samples {
		t.Run(name, func(t *testing.T) {
			require.True(t, SetContextInfo(msg, info), "should accept %s", name)
			require.NotNil(t, ContextInfoOf(msg), "should read back %s", name)
			assert.True(t, ContextInfoOf(msg).GetIsForwarded())
		})
	}
}

func TestSetContextInfo_RejectsWhatItCannotCarry(t *testing.T) {
	// Plain text is the deliberate one: it has no ContextInfo field, which is
	// why forwarding promotes it to an extended text message first.
	assert.False(t, SetContextInfo(&waE2E.Message{Conversation: proto.String("x")}, &waE2E.ContextInfo{}))
	assert.False(t, SetContextInfo(&waE2E.Message{}, &waE2E.ContextInfo{}))
	assert.False(t, SetContextInfo(nil, &waE2E.ContextInfo{}))
}
