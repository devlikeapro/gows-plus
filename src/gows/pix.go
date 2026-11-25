package gows

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/google/uuid"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

const (
	PixButtonName         = "payment_info"
	PixPaymentTypeStatic  = "pix_static_code"
	PixDefaultCurrency    = "BRL"
	pixReferenceIDLength  = 11
	pixReferenceIDCharset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

var PixKeyTypes = []string{"CPF", "CNPJ", "PHONE", "EMAIL", "EVP"}

var ErrInvalidPix = errors.New("invalid pix message")

// PixMessage is a static PIX key the recipient can copy from a native button
type PixMessage struct {
	MerchantName string
	Key          string
	KeyType      string
	Currency     string
	ReferenceID  string
}

type pixAmount struct {
	Value  int `json:"value"`
	Offset int `json:"offset"`
}

type pixOrderItem struct {
	Name       string    `json:"name"`
	Amount     pixAmount `json:"amount"`
	Quantity   int       `json:"quantity"`
	SaleAmount pixAmount `json:"sale_amount"`
}

type pixOrder struct {
	Status    string         `json:"status"`
	Subtotal  pixAmount      `json:"subtotal"`
	OrderType string         `json:"order_type"`
	Items     []pixOrderItem `json:"items"`
}

type pixStaticCode struct {
	MerchantName string `json:"merchant_name"`
	Key          string `json:"key"`
	KeyType      string `json:"key_type"`
}

type pixPaymentSetting struct {
	Type          string         `json:"type"`
	PixStaticCode *pixStaticCode `json:"pix_static_code"`
}

type pixButtonParams struct {
	Currency           string              `json:"currency"`
	TotalAmount        pixAmount           `json:"total_amount"`
	ReferenceID        string              `json:"reference_id"`
	Type               string              `json:"type"`
	Order              pixOrder            `json:"order"`
	PaymentSettings    []pixPaymentSetting `json:"payment_settings"`
	SharePaymentStatus bool                `json:"share_payment_status"`
}

type pixMessageParams struct {
	From       string `json:"from"`
	TemplateID string `json:"templateId"`
}

func normalizePixKeyType(keyType string) (string, bool) {
	normalized := strings.ToUpper(strings.TrimSpace(keyType))
	for _, t := range PixKeyTypes {
		if t == normalized {
			return normalized, true
		}
	}
	return "", false
}

func generatePixReferenceID() (string, error) {
	max := big.NewInt(int64(len(pixReferenceIDCharset)))
	var sb strings.Builder
	sb.Grow(pixReferenceIDLength)
	for i := 0; i < pixReferenceIDLength; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		sb.WriteByte(pixReferenceIDCharset[n.Int64()])
	}
	return sb.String(), nil
}

// BuildPixMessage builds a native flow "payment_info" message with a static PIX key
func BuildPixMessage(pix PixMessage, contextInfo *waE2E.ContextInfo) (*waE2E.Message, error) {
	merchantName := strings.TrimSpace(pix.MerchantName)
	if merchantName == "" {
		return nil, fmt.Errorf("%w: merchantName is required", ErrInvalidPix)
	}
	key := strings.TrimSpace(pix.Key)
	if key == "" {
		return nil, fmt.Errorf("%w: key is required", ErrInvalidPix)
	}
	keyType, ok := normalizePixKeyType(pix.KeyType)
	if !ok {
		return nil, fmt.Errorf("%w: keyType must be one of %s, got '%s'", ErrInvalidPix, strings.Join(PixKeyTypes, ", "), pix.KeyType)
	}
	currency := strings.ToUpper(strings.TrimSpace(pix.Currency))
	if currency == "" {
		currency = PixDefaultCurrency
	}
	referenceID := strings.TrimSpace(pix.ReferenceID)
	if referenceID == "" {
		var err error
		referenceID, err = generatePixReferenceID()
		if err != nil {
			return nil, fmt.Errorf("failed to generate pix reference id: %w", err)
		}
	}

	zero := pixAmount{Value: 0, Offset: 100}
	params := pixButtonParams{
		Currency:    currency,
		TotalAmount: zero,
		ReferenceID: referenceID,
		Type:        "physical-goods",
		Order: pixOrder{
			Status:    "pending",
			Subtotal:  zero,
			OrderType: "ORDER",
			Items: []pixOrderItem{{
				Name:       "",
				Amount:     zero,
				Quantity:   0,
				SaleAmount: zero,
			}},
		},
		PaymentSettings: []pixPaymentSetting{{
			Type: PixPaymentTypeStatic,
			PixStaticCode: &pixStaticCode{
				MerchantName: merchantName,
				Key:          key,
				KeyType:      keyType,
			},
		}},
		SharePaymentStatus: false,
	}
	buttonParams, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal pix button params: %w", err)
	}
	messageParams, err := json.Marshal(pixMessageParams{From: "api", TemplateID: uuid.NewString()})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal pix message params: %w", err)
	}

	interactive := &waE2E.InteractiveMessage{
		InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
			NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
				Buttons: []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{{
					Name:             proto.String(PixButtonName),
					ButtonParamsJSON: proto.String(string(buttonParams)),
				}},
				MessageParamsJSON: proto.String(string(messageParams)),
			},
		},
		ContextInfo: contextInfo,
	}
	// No view-once wrapper - clients drop wrapped payment_info messages
	return &waE2E.Message{InteractiveMessage: interactive}, nil
}

// IsPixMessage reports whether msg is a native flow "payment_info" message
func IsPixMessage(msg *waE2E.Message) bool {
	buttons := msg.GetInteractiveMessage().GetNativeFlowMessage().GetButtons()
	return len(buttons) > 0 && buttons[0].GetName() == PixButtonName
}

// PixBizNode is the stanza node required for clients to render a "payment_info" message
func PixBizNode() waBinary.Node {
	return waBinary.Node{
		Tag:   "biz",
		Attrs: waBinary.Attrs{"native_flow_name": PixButtonName},
	}
}
