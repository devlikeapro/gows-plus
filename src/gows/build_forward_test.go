package gows

import (
	"reflect"
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

// The caller's copy must come back untouched. Defensive today - the SQL store
// unmarshals a fresh object per read - but the cost is one clone and the cure
// for getting it wrong later is a corrupted stored message.
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

// SetContextInfo is the writing half of ContextInfoOf, and the two have to agree
// on every type. Reading one the other cannot write means a forward refused for
// no reason; writing one the other cannot read means the forwarding score is
// read back as zero and the chain silently stops counting.
//
// Walked by reflection rather than listed by hand: a list only re-checks what
// its author already knew, so a type added to one function and forgotten in the
// other would keep the test green.
func TestContextInfoHelpersStayInSync(t *testing.T) {
	msgType := reflect.TypeOf(waE2E.Message{})
	carriers := 0
	for i := 0; i < msgType.NumField(); i++ {
		field := msgType.Field(i)
		if field.Type.Kind() != reflect.Ptr || field.Type.Elem().Kind() != reflect.Struct {
			continue
		}
		if _, ok := field.Type.Elem().FieldByName("ContextInfo"); !ok {
			continue
		}
		carriers++
		t.Run(field.Name, func(t *testing.T) {
			msg := &waE2E.Message{}
			reflect.ValueOf(msg).Elem().Field(i).Set(reflect.New(field.Type.Elem()))

			written := SetContextInfo(msg, &waE2E.ContextInfo{IsForwarded: proto.Bool(true)})
			read := ContextInfoOf(msg) != nil
			if written != read {
				t.Fatalf(
					"SetContextInfo=%v but ContextInfoOf=%v for %s - the two lists disagree",
					written, read, field.Name,
				)
			}
			if !written {
				t.Skipf("%s can carry a ContextInfo but is not forwardable yet", field.Name)
			}
			assert.True(t, ContextInfoOf(msg).GetIsForwarded())
		})
	}
	// Without this the test passes by walking nothing at all, which is how a
	// broken reflection walk looks exactly like a clean run.
	require.Greater(t, carriers, 15, "reflection found almost no ContextInfo carriers")
}

func TestSetContextInfo_RejectsWhatItCannotCarry(t *testing.T) {
	// Plain text is the deliberate one: it has no ContextInfo field, which is
	// why forwarding promotes it to an extended text message first.
	assert.False(t, SetContextInfo(&waE2E.Message{Conversation: proto.String("x")}, &waE2E.ContextInfo{}))
	assert.False(t, SetContextInfo(&waE2E.Message{}, &waE2E.ContextInfo{}))
	assert.False(t, SetContextInfo(nil, &waE2E.ContextInfo{}))
}

// WhatsApp offers no way to forward a poll, so the API must not invent one.
// Sending it anyway would produce a poll whose votes nobody can decrypt, and
// report success - the expensive kind of failure.
func TestBuildForwardedMessage_RefusesPolls(t *testing.T) {
	for name, msg := range map[string]*waE2E.Message{
		"v1": {PollCreationMessage: &waE2E.PollCreationMessage{Name: proto.String("p")}},
		"v2": {PollCreationMessageV2: &waE2E.PollCreationMessage{Name: proto.String("p")}},
		"v3": {PollCreationMessageV3: &waE2E.PollCreationMessage{Name: proto.String("p")}},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := BuildForwardedMessage(incoming(msg), nil, false)
			assert.ErrorIs(t, err, ErrCannotForwardPoll)
			assert.Nil(t, out)
		})
	}
}

// Everything else keeps losing it: the device list and the secret belong to the
// delivery the message came from.
func TestBuildForwardedMessage_DropsMessageContextInfo(t *testing.T) {
	original := incoming(&waE2E.Message{
		ImageMessage:       &waE2E.ImageMessage{MediaKey: []byte("k")},
		MessageContextInfo: &waE2E.MessageContextInfo{MessageSecret: []byte("secret")},
	})

	out, err := BuildForwardedMessage(original, nil, false)

	require.NoError(t, err)
	assert.Nil(t, out.MessageContextInfo)
}
