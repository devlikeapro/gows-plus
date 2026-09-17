package gows

import (
	"context"
	"errors"
	"fmt"
	"github.com/samber/lo"
	"github.com/samber/lo/mutable"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"time"
)

// SendStatusMessage sends a status message to a Broadcast list.
func (gows *GoWS) SendStatusMessage(ctx context.Context, to types.JID, msg *waE2E.Message, extra whatsmeow.SendRequestExtra) (*whatsmeow.SendResponse, error) {
	var err error

	allParticipants := extra.Participants
	if len(allParticipants) == 0 {
		// No participants provided, fetch them
		allParticipants, err = gows.int.GetBroadcastListParticipants(ctx, to)
		if err != nil {
			return nil, err
		}
		// so we have ownId first
		mutable.Reverse(allParticipants)
	}

	// Filter out only the right participants
	validParticipants := lo.Filter(allParticipants, func(p types.JID, _ int) bool {
		return p.Server == types.DefaultUserServer
	})

	participantsBatchSize := statusParticipantsBatchSize
	if len(extra.Participants) > 0 {
		// If participants are provided, use the batch size of the participants
		participantsBatchSize = len(extra.Participants)
	}

	batches := lo.Chunk(validParticipants, participantsBatchSize)
	if extra.ID == "" {
		extra.ID = gows.Client.GenerateMessageID()
	}

	errs := make([]error, 0)
	ignored := len(allParticipants) - len(validParticipants)
	gows.Log.Infof(
		"Sending status message (%s) in %d batches - %d participants in total, %d per batch, %d ignored",
		extra.ID,
		len(batches),
		len(validParticipants),
		participantsBatchSize,
		ignored,
	)
	for index, participants := range batches {
		batchExtra := extra
		batchExtra.Participants = participants
		// Give the server room to acknowledge a large batch. Without this the
		// whatsmeow default (75s) applies, and a batch that is actually delivered
		// is reported as "timed out waiting for message send response".
		if statusBatchTimeout > 0 {
			batchExtra.Timeout = statusBatchTimeout
		}

		_, err := gows.Client.SendMessage(ctx, to, msg, batchExtra)
		if err != nil {
			gows.Log.Errorf("Failed to send message (%s) to (batch %d/%d): %v", extra.ID, index+1, len(batches), err)
			errs = append(errs, fmt.Errorf("batch %d: %w", index+1, err))
		} else {
			gows.Log.Infof("Sending status message (%s) to %d participants (batch %d/%d) - success", extra.ID, len(participants), index+1, len(batches))
		}
	}

	if len(errs) > 0 {
		err = errors.Join(errs...)
		gows.Log.Errorf("Failed to send status message (%s): %v", extra.ID, err)
		return nil, err
	}

	gows.Log.Infof("Sending status message (%s) - success", extra.ID)

	result := &whatsmeow.SendResponse{
		ID:        extra.ID,
		Timestamp: time.Now(),
	}
	return result, nil
}
