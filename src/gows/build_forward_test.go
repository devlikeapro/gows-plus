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

func TestBuildForwardedMessage_OwnMessageIsNotMarked(t *testing.T) {
	original := outgoing(&waE2E.Message{Conversation: proto.String("mine")})

	out, err := BuildForwardedMessage(original, nil, false)

	require.NoError(t, err)
	assert.False(t, out.ExtendedTextMessage.ContextInfo.GetIsForwarded())
	assert.EqualValues(t, 0, out.ExtendedTextMessage.ContextInfo.GetForwardingScore())
}

func TestBuildForwardedMessage_OwnMessageWithForce(t *testing.T) {
	original := outgoing(&waE2E.Message{Conversation: proto.String("mine")})

	out, err := BuildForwardedMessage(original, nil, true)

	require.NoError(t, err)
	assert.True(t, out.ExtendedTextMessage.ContextInfo.GetIsForwarded())
	assert.EqualValues(t, 1, out.ExtendedTextMessage.ContextInfo.GetForwardingScore())
}

func TestBuildForwardedMessage_KeepsDestinationContext(t *testing.T) {
	original := incoming(&waE2E.Message{Conversation: proto.String("hi")})
	base := &waE2E.ContextInfo{Expiration: proto.Uint32(604800)}

	out, err := BuildForwardedMessage(original, base, false)

	require.NoError(t, err)
	ci := out.ExtendedTextMessage.ContextInfo
	assert.EqualValues(t, 604800, ci.GetExpiration())
	assert.True(t, ci.GetIsForwarded())
}

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

// ContextInfoOf and SetContextInfo must cover the same types - walked by reflection so a new type can't be missed in one of them
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
	require.Greater(t, carriers, 15, "reflection found almost no ContextInfo carriers")
}

func TestSetContextInfo_RejectsWhatItCannotCarry(t *testing.T) {
	assert.False(t, SetContextInfo(&waE2E.Message{Conversation: proto.String("x")}, &waE2E.ContextInfo{}))
	assert.False(t, SetContextInfo(&waE2E.Message{}, &waE2E.ContextInfo{}))
	assert.False(t, SetContextInfo(nil, &waE2E.ContextInfo{}))
}

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

func TestBuildForwardedMessage_DropsMessageContextInfo(t *testing.T) {
	original := incoming(&waE2E.Message{
		ImageMessage:       &waE2E.ImageMessage{MediaKey: []byte("k")},
		MessageContextInfo: &waE2E.MessageContextInfo{MessageSecret: []byte("secret")},
	})

	out, err := BuildForwardedMessage(original, nil, false)

	require.NoError(t, err)
	assert.Nil(t, out.MessageContextInfo)
}
