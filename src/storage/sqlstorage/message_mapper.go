package sqlstorage

import (
	"encoding/json"
	"time"

	"github.com/devlikeapro/gows/storage"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/encoding/protojson"
)

func isNullJson(data []byte) bool {
	if len(data) != 4 {
		return false
	}
	// null
	return data[0] == 'n' && data[1] == 'u' && data[2] == 'l' && data[3] == 'l'
}

type MessageMapper struct {
}

var messageMapper = &MessageMapper{}

// Stored JSON may contain fields renamed or removed by later WhatsApp proto updates,
// so unknown fields must not fail the read.
// e.g. faviconMMSMetadata -> faviconMmsMetadata
var pjson = protojson.UnmarshalOptions{DiscardUnknown: true}

var _ Mapper[storage.StoredMessage] = (*MessageMapper)(nil)

func (f *MessageMapper) ToFields(entity *storage.StoredMessage) map[string]interface{} {
	return map[string]interface{}{
		"id":        entity.Info.ID,
		"jid":       entity.Info.Chat,
		"from_me":   entity.Info.IsFromMe,
		"timestamp": entity.Info.Timestamp,
		"is_real":   entity.IsReal,
	}
}

// messageTemp is a reusable struct to reduce allocation
// messageTemp is a reusable struct to reduce allocations
type messageTemp struct {
	Info                  types.MessageInfo             `json:"Info"`
	Message               json.RawMessage               `json:"Message"`
	IsEphemeral           bool                          `json:"IsEphemeral"`
	IsViewOnce            bool                          `json:"IsViewOnce"`
	IsViewOnceV2          bool                          `json:"IsViewOnceV2"`
	IsViewOnceV2Extension bool                          `json:"IsViewOnceV2Extension"`
	IsDocumentWithCaption bool                          `json:"IsDocumentWithCaption"`
	IsLottieSticker       bool                          `json:"IsLottieSticker"`
	IsEdit                bool                          `json:"IsEdit"`
	SourceWebMsg          json.RawMessage               `json:"SourceWebMsg"`
	UnavailableRequestID  string                        `json:"UnavailableRequestID"`
	RetryCount            int                           `json:"RetryCount"`
	NewsletterMeta        *events.NewsletterMessageMeta `json:"NewsletterMeta"`
	RawMessage            json.RawMessage               `json:"RawMessage"`
	Status                storage.Status                `json:"Status"`
	IsReal                bool                          `json:"IsReal"`
	EditedAt              *time.Time                    `json:"EditedAt,omitempty"`
}

func (f *MessageMapper) Marshal(msg *storage.StoredMessage) ([]byte, error) {
	// Use a local variable to avoid heap allocation
	var temp messageTemp

	if msg.Message != nil && msg.Message.Message != nil {
		var err error
		temp.Message, err = protojson.Marshal(msg.Message.Message)
		if err != nil {
			return nil, err
		}
	}

	if msg.RawMessage != nil {
		var err error
		temp.RawMessage, err = protojson.Marshal(msg.RawMessage)
		if err != nil {
			return nil, err
		}
	}

	if msg.SourceWebMsg != nil {
		var err error
		temp.SourceWebMsg, err = protojson.Marshal(msg.SourceWebMsg)
		if err != nil {
			return nil, err
		}
	}

	temp.Info = msg.Info
	temp.IsEphemeral = msg.IsEphemeral
	temp.IsViewOnce = msg.IsViewOnce
	temp.IsViewOnceV2 = msg.IsViewOnceV2
	temp.IsViewOnceV2Extension = msg.IsViewOnceV2Extension
	temp.IsDocumentWithCaption = msg.IsDocumentWithCaption
	temp.IsLottieSticker = msg.IsLottieSticker
	temp.IsEdit = msg.IsEdit
	temp.UnavailableRequestID = msg.UnavailableRequestID
	temp.RetryCount = msg.RetryCount
	temp.NewsletterMeta = msg.NewsletterMeta
	if msg.Status != nil {
		temp.Status = *msg.Status
	}
	temp.IsReal = msg.IsReal
	if !msg.EditedAt.IsZero() {
		editedAt := msg.EditedAt
		temp.EditedAt = &editedAt
	}

	return json.Marshal(temp)
}

func (f *MessageMapper) Unmarshal(data []byte, msg *storage.StoredMessage) error {
	// Use the messageTemp struct to reduce allocations
	var temp messageTemp

	// Unmarshal into the temporary structure
	temp.IsReal = true
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	// Initialize Message only if needed
	if msg.Message == nil {
		msg.Message = &events.Message{}
	}

	// Assign values to msg
	msg.Info = temp.Info
	msg.IsEphemeral = temp.IsEphemeral
	msg.IsViewOnce = temp.IsViewOnce
	msg.IsViewOnceV2 = temp.IsViewOnceV2
	msg.IsViewOnceV2Extension = temp.IsViewOnceV2Extension
	msg.IsDocumentWithCaption = temp.IsDocumentWithCaption
	msg.IsLottieSticker = temp.IsLottieSticker
	msg.IsEdit = temp.IsEdit
	msg.UnavailableRequestID = temp.UnavailableRequestID
	msg.RetryCount = temp.RetryCount
	msg.NewsletterMeta = temp.NewsletterMeta

	// Create a copy of the Status value to avoid storing a pointer to a local variable
	if msg.Status == nil {
		status := temp.Status
		msg.Status = &status
	} else {
		*msg.Status = temp.Status
	}

	msg.IsReal = temp.IsReal
	msg.EditedAt = time.Time{}
	if temp.EditedAt != nil {
		msg.EditedAt = *temp.EditedAt
	}

	// Unmarshal Message if present
	if !isNullJson(temp.Message) {
		if msg.Message.Message == nil {
			msg.Message.Message = &waProto.Message{}
		}
		if err := pjson.Unmarshal(temp.Message, msg.Message.Message); err != nil {
			return err
		}
	}

	// Unmarshal RawMessage if present
	if !isNullJson(temp.RawMessage) {
		if msg.RawMessage == nil {
			msg.RawMessage = &waProto.Message{}
		}
		if err := pjson.Unmarshal(temp.RawMessage, msg.RawMessage); err != nil {
			return err
		}
	}

	// Unmarshal SourceWebMsg if present
	if !isNullJson(temp.SourceWebMsg) {
		if msg.SourceWebMsg == nil {
			msg.SourceWebMsg = &waProto.WebMessageInfo{}
		}
		if err := pjson.Unmarshal(temp.SourceWebMsg, msg.SourceWebMsg); err != nil {
			return err
		}
	}

	return nil
}
