package gows

import (
	"bytes"
	"context"
	"fmt"
	"github.com/samber/lo"
	"go.mau.fi/util/random"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// BuildPollCreationV3 builds a poll creation message (v3) with the given poll name, options and maximum number of selections.
// The built message can be sent normally using Client.SendMessage.
//
//	resp, err := cli.SendMessage(context.Background(), chat, cli.BuildPollCreation("meow?", []string{"yes", "no"}, 1))
func (gows *GoWS) BuildPollCreationV3(name string, optionNames []string, selectableOptionCount int) *waE2E.Message {
	msgSecret := random.Bytes(32)
	if selectableOptionCount < 0 || selectableOptionCount > len(optionNames) {
		selectableOptionCount = 0
	}
	options := make([]*waE2E.PollCreationMessage_Option, len(optionNames))
	for i, option := range optionNames {
		options[i] = &waE2E.PollCreationMessage_Option{OptionName: proto.String(option)}
	}
	pollCreationMessage := &waE2E.PollCreationMessage{
		Name:                   proto.String(name),
		Options:                options,
		SelectableOptionsCount: proto.Uint32(uint32(selectableOptionCount)),
		PollContentType:        waE2E.PollContentType_TEXT.Enum(),
	}
	if selectableOptionCount == 1 {
		return &waE2E.Message{
			PollCreationMessageV3: pollCreationMessage,
			MessageContextInfo: &waE2E.MessageContextInfo{
				MessageSecret: msgSecret,
			},
		}
	}
	return &waE2E.Message{
		PollCreationMessage: pollCreationMessage,
		MessageContextInfo: &waE2E.MessageContextInfo{
			MessageSecret: msgSecret,
		},
	}
}

// PollCreationOf returns the poll of a poll creation message, whichever version carries it.
// WhatsApp sends polls as pollCreationMessage (v1), v2, v3 and - for polls with an end time
// or hidden voter names - v5 / v6; reading only v1/v3 leaves the options empty and every
// vote on the other versions fails with "failed to find hash in option hashes".
func PollCreationOf(msg *waE2E.Message) *waE2E.PollCreationMessage {
	if msg == nil {
		return nil
	}
	for _, p := range []*waE2E.PollCreationMessage{
		msg.GetPollCreationMessage(),
		msg.GetPollCreationMessageV2(),
		msg.GetPollCreationMessageV3(),
		msg.GetPollCreationMessageV5(),
		msg.GetPollCreationMessageV6(),
	} {
		if p != nil {
			return p
		}
	}
	return nil
}

func (gows *GoWS) extractPollVotes(ctx context.Context, msg *events.Message) (*[]string, error) {
	// poll vote has hash inside selected options
	// we need to decrypt it, find the original message,
	// and lookup it's hash in the originally send options
	pollVote, err := gows.DecryptPollVote(ctx, msg)
	if err != nil {
		return nil, err
	}
	// Get a saved message
	pollUpdate := msg.Message.GetPollUpdateMessage()
	storedMessage, err := gows.Storage.Messages.GetMessageWithRetries(pollUpdate.GetPollCreationMessageKey().GetID())
	if err != nil {
		return nil, err
	}

	// Creation message - any version (v1, v2, v3, v5, v6)
	pollCreationMessage := PollCreationOf(storedMessage.Message.Message)
	if pollCreationMessage == nil {
		return nil, fmt.Errorf("stored message %s is not a poll creation", pollUpdate.GetPollCreationMessageKey().GetID())
	}

	// Get options-hashes
	options := pollCreationMessage.GetOptions()
	optionNames := make([]string, len(options))
	for i, option := range options {
		optionNames[i] = *option.OptionName
	}
	hashes := whatsmeow.HashPollOptions(optionNames)

	// find hash in hashes by index to populate votes
	votes := make([]string, 0)
	for _, hash := range pollVote.GetSelectedOptions() {
		_, idx, ok := lo.FindIndexOf(
			hashes,
			func(v []byte) bool {
				return bytes.Equal(v, hash)
			})
		if !ok {
			return nil, fmt.Errorf("failed to find hash in option hashes - %v", hash)
		}
		votes = append(votes, optionNames[idx])
	}
	return &votes, nil
}

func (gows *GoWS) handleEncPollVote(ctx context.Context, msg *events.Message) {
	votes, err := gows.extractPollVotes(ctx, msg)
	if err != nil {
		gows.Log.Errorf("Failed to extract from encrypted poll vote: %v", err)
	}
	data := &PollVoteEvent{
		Message: msg,
		Votes:   votes,
	}
	gows.emitEvent(data)
}
