package gows

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func pixButton(t *testing.T, msg *waE2E.Message) *waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton {
	t.Helper()
	assert.Nil(t, msg.ViewOnceMessage, "payment_info must not be wrapped in view-once")
	interactive := msg.GetInteractiveMessage()
	require.NotNil(t, interactive)
	flow := interactive.GetNativeFlowMessage()
	require.NotNil(t, flow)
	require.Len(t, flow.GetButtons(), 1)
	return flow.GetButtons()[0]
}

func TestBuildPixMessage_Structure(t *testing.T) {
	info := &waE2E.ContextInfo{StanzaID: proto.String("quoted")}
	msg, err := BuildPixMessage(PixMessage{
		MerchantName: "Loja Teste",
		Key:          "12345678901",
		KeyType:      "cpf",
	}, info)
	require.NoError(t, err)

	button := pixButton(t, msg)
	assert.Equal(t, "payment_info", button.GetName())

	var params map[string]any
	require.NoError(t, json.Unmarshal([]byte(button.GetButtonParamsJSON()), &params))
	assert.Equal(t, "BRL", params["currency"])
	assert.Equal(t, false, params["share_payment_status"])
	assert.Regexp(t, regexp.MustCompile(`^[A-Z0-9]{11}$`), params["reference_id"])

	settings := params["payment_settings"].([]any)
	require.Len(t, settings, 1)
	setting := settings[0].(map[string]any)
	assert.Equal(t, "pix_static_code", setting["type"])
	static := setting["pix_static_code"].(map[string]any)
	assert.Equal(t, "Loja Teste", static["merchant_name"])
	assert.Equal(t, "12345678901", static["key"])
	assert.Equal(t, "CPF", static["key_type"])

	var messageParams map[string]any
	flow := msg.GetInteractiveMessage().GetNativeFlowMessage()
	require.NoError(t, json.Unmarshal([]byte(flow.GetMessageParamsJSON()), &messageParams))
	assert.Equal(t, "api", messageParams["from"])
	assert.NotEmpty(t, messageParams["templateId"])

	assert.Same(t, info, msg.GetInteractiveMessage().GetContextInfo())
}

func TestBuildPixMessage_KeepsProvidedValues(t *testing.T) {
	msg, err := BuildPixMessage(PixMessage{
		MerchantName: "Loja",
		Key:          "a@b.com",
		KeyType:      "EMAIL",
		Currency:     "brl",
		ReferenceID:  "REF123",
	}, nil)
	require.NoError(t, err)

	var params pixButtonParams
	require.NoError(t, json.Unmarshal([]byte(pixButton(t, msg).GetButtonParamsJSON()), &params))
	assert.Equal(t, "BRL", params.Currency)
	assert.Equal(t, "REF123", params.ReferenceID)
}

func TestBuildPixMessage_Validation(t *testing.T) {
	cases := map[string]PixMessage{
		"no merchant": {Key: "k", KeyType: "EVP"},
		"no key":      {MerchantName: "m", KeyType: "EVP"},
		"bad type":    {MerchantName: "m", Key: "k", KeyType: "IBAN"},
		"no type":     {MerchantName: "m", Key: "k"},
	}
	for name, pix := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := BuildPixMessage(pix, nil)
			assert.ErrorIs(t, err, ErrInvalidPix)
		})
	}
}

func TestBuildForwardedMessage_Interactive(t *testing.T) {
	built, err := BuildPixMessage(PixMessage{MerchantName: "m", Key: "k", KeyType: "EVP"}, nil)
	require.NoError(t, err)
	original := incoming(built)

	out, err := BuildForwardedMessage(original, nil, false)

	require.NoError(t, err)
	require.NotNil(t, out.InteractiveMessage)
	assert.True(t, out.InteractiveMessage.GetContextInfo().GetIsForwarded())
	assert.True(t, IsPixMessage(out))
}

func TestIsPixMessage(t *testing.T) {
	built, err := BuildPixMessage(PixMessage{MerchantName: "m", Key: "k", KeyType: "EVP"}, nil)
	require.NoError(t, err)
	assert.True(t, IsPixMessage(built))
	assert.False(t, IsPixMessage(&waE2E.Message{Conversation: proto.String("hi")}))
	assert.False(t, IsPixMessage(nil))
}

func TestPixBizNode(t *testing.T) {
	node := PixBizNode()
	assert.Equal(t, "biz", node.Tag)
	assert.Equal(t, "payment_info", node.Attrs["native_flow_name"])
	assert.Nil(t, node.Content)
}
