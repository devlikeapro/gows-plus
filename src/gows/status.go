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
	"strings"
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

	// Always batch by the configured size, including when the caller supplied the
	// participant list. Using len(extra.Participants) as the batch size sent an
	// explicit list as one single batch - precisely the large send that has to be
	// split.
	participantsBatchSize := statusParticipantsBatchSize

	batches := lo.Chunk(validParticipants, participantsBatchSize)
	if extra.ID == "" {
		extra.ID = gows.Client.GenerateMessageID()
	}

	errs := make([]error, 0)
	succeeded := 0
	deliveredBatches := make([]int, 0, len(batches))
	failedBatches := make([]int, 0)
	ignored := len(allParticipants) - len(validParticipants)
	gows.Log.Infof(
		"Sending status message (%s) in %d batches - %d participants in total, %d per batch, %d ignored (timeout=%s, delay=%s, retries=%d)",
		extra.ID,
		len(batches),
		len(validParticipants),
		participantsBatchSize,
		ignored,
		statusBatchTimeout,
		statusBatchDelay,
		statusBatchMaxRetries,
	)
	for index, participants := range batches {
		// Steady cadence rather than a burst of back-to-back stanzas. Not before
		// the first batch, there is nothing to space it from.
		if index > 0 {
			if err := sleepCtx(ctx, statusBatchDelay); err != nil {
				return nil, err
			}
		}

		batchExtra := extra
		batchExtra.Participants = participants
		// Give the server room to acknowledge a large batch. Without this the
		// whatsmeow default (75s) applies, and a batch that is actually delivered
		// is reported as "timed out waiting for message send response".
		if statusBatchTimeout > 0 {
			batchExtra.Timeout = statusBatchTimeout
		}

		batchErr := gows.sendStatusBatchWithRetry(ctx, to, msg, batchExtra, index+1, len(batches))
		if batchErr != nil {
			gows.Log.Errorf("Failed to send message (%s) to (batch %d/%d): %v", extra.ID, index+1, len(batches), batchErr)
			errs = append(errs, fmt.Errorf("batch %d: %w", index+1, batchErr))
			failedBatches = append(failedBatches, index+1)
		} else {
			succeeded++
			deliveredBatches = append(deliveredBatches, index+1)
			gows.Log.Infof("Sending status message (%s) to %d participants (batch %d/%d) - success", extra.ID, len(participants), index+1, len(batches))
		}
	}

	// Report at batch granularity, so a broadcast is never an opaque
	// success/failure. Logged before the total-failure return below, so a send
	// that reached nobody still records what it attempted.
	gows.Log.Infof(
		"Status (%s) delivery report: %d/%d batches delivered (ok=%v, failed=%v), %d ignored",
		extra.ID, len(deliveredBatches), len(batches), deliveredBatches, failedBatches, ignored,
	)

	// Best effort: fail the call only when nothing got through. A status that
	// reached at least one batch is already live on WhatsApp, so reporting a
	// partial failure as an error only pushes the caller to send the whole thing
	// again - which posts the status a second time.
	if succeeded == 0 && len(errs) > 0 {
		err = errors.Join(errs...)
		gows.Log.Errorf("Failed to send status message (%s): %v", extra.ID, err)
		return nil, err
	}

	if len(errs) > 0 {
		gows.Log.Warnf(
			"Status message (%s) partially delivered: %d/%d batches ok",
			extra.ID, succeeded, len(batches),
		)
	} else {
		gows.Log.Infof("Sending status message (%s) - success", extra.ID)
	}

	result := &whatsmeow.SendResponse{
		ID:        extra.ID,
		Timestamp: time.Now(),
	}
	return result, nil
}

// sendStatusBatchWithRetry sends one batch, retrying transient failures with an
// exponential backoff.
//
// Retrying is safe: extra.ID is generated once for the whole status and every
// batch and every attempt reuses it, and WhatsApp deduplicates by (sender,
// message ID). A recipient that already received the batch does not see the
// status twice.
func (gows *GoWS) sendStatusBatchWithRetry(
	ctx context.Context,
	to types.JID,
	msg *waE2E.Message,
	batchExtra whatsmeow.SendRequestExtra,
	batchNum, batchTotal int,
) error {
	var lastErr error
	backoff := statusBatchRetryBackoff
	for attempt := 0; attempt <= statusBatchMaxRetries; attempt++ {
		if attempt > 0 {
			gows.Log.Warnf(
				"Retrying status batch %d/%d (attempt %d/%d) after %s - previous error: %v",
				batchNum, batchTotal, attempt, statusBatchMaxRetries, backoff, lastErr,
			)
			if err := sleepCtx(ctx, backoff); err != nil {
				return err
			}
			backoff *= 3
		}

		_, err := gows.Client.SendMessage(ctx, to, msg, batchExtra)
		if err == nil {
			return nil
		}
		lastErr = err
		if !isRetryableStatusErr(err) {
			return err
		}
	}
	return lastErr
}

// isRetryableStatusErr reports whether a failed status batch is worth sending
// again. Only two conditions are transient: an ack timeout, and an explicit 429
// rate limit. Any other server error - a permanent rejection, a capping nack -
// would just burn the retry budget and delay the remaining batches.
//
// whatsmeow renders server errors as "server returned error <code>" with the
// code appended as text, so matching the code means matching the suffix; there
// is no structured error to inspect.
func isRetryableStatusErr(err error) bool {
	if errors.Is(err, whatsmeow.ErrMessageTimedOut) {
		return true
	}
	return errors.Is(err, whatsmeow.ErrServerReturnedError) &&
		strings.HasSuffix(err.Error(), " 429")
}

// sleepCtx waits for d, returning early if the context is cancelled.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
