package gows

import (
	"context"
	"encoding/json"

	"go.mau.fi/util/jsontime"
)

// The same MEX (GraphQL) query WhatsApp Web runs to read the account's new-chat
// message capping (FetchNewChatMessageCappingInfo -> xwa2_message_capping_info).
const queryFetchMessageCapping = "24503548349331633"

// MessageCappingTypeNewChatThread is the capping bucket WhatsApp Web reads for
// the per-cycle quota on starting new individual chats.
const MessageCappingTypeNewChatThread = "INDIVIDUAL_NEW_CHAT_THREAD"

// MessageCapping is the per-cycle quota WhatsApp applies to starting new chat
// threads - the volume counterpart of the reachout timelock. Unlike the
// timelock, WhatsApp has no whatsmeow event type for it, so it is defined here
// and fetched on demand. TotalQuota is -1 when the account has no cap.
type MessageCapping struct {
	CappingStatus string              `json:"capping_status,omitempty"`
	TotalQuota    int                 `json:"total_quota"`
	UsedQuota     int                 `json:"used_quota"`
	CycleStart    jsontime.UnixString `json:"cycle_start_timestamp,omitzero"`
	CycleEnd      jsontime.UnixString `json:"cycle_end_timestamp,omitzero"`
	MvStatus      string              `json:"mv_status,omitempty"`
	OteStatus     string              `json:"ote_status,omitempty"`
}

type respFetchMessageCapping struct {
	Capping *MessageCapping `json:"xwa2_message_capping_info"`
}

// FetchMessageCapping fetches the account's current new-chat message capping
// (quota) for the given type. There is no push notification for it that
// whatsmeow handles, so this active fetch is the only way to read it. If the
// response has no capping data, a zero-value MessageCapping is returned instead
// of an error.
func (gows *GoWS) FetchMessageCapping(ctx context.Context, cappingType string) (*MessageCapping, error) {
	data, err := gows.int.SendMexIQ(ctx, queryFetchMessageCapping, Map{
		"input": Map{"type": cappingType},
	})
	if err != nil {
		return nil, err
	}
	if data == nil {
		return &MessageCapping{}, nil
	}
	var respData respFetchMessageCapping
	err = json.Unmarshal(data, &respData)
	if err != nil {
		return nil, err
	}
	if respData.Capping == nil {
		return &MessageCapping{}, nil
	}
	return respData.Capping, nil
}
