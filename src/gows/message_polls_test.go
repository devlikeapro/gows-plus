package gows

import (
	"bytes"
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func poll(options ...string) *waE2E.PollCreationMessage {
	opts := make([]*waE2E.PollCreationMessage_Option, len(options))
	for i, o := range options {
		opts[i] = &waE2E.PollCreationMessage_Option{OptionName: proto.String(o)}
	}
	return &waE2E.PollCreationMessage{Name: proto.String("Where?"), Options: opts}
}

func TestPollCreationOf_EveryVersion(t *testing.T) {
	p := poll("Beach", "Pool")
	cases := map[string]*waE2E.Message{
		"v1": {PollCreationMessage: p},
		"v2": {PollCreationMessageV2: p},
		"v3": {PollCreationMessageV3: p},
		"v5": {PollCreationMessageV5: p},
		"v6": {PollCreationMessageV6: p},
	}
	for name, msg := range cases {
		if got := PollCreationOf(msg); got != p {
			t.Errorf("%s: poll not found", name)
		}
		if !isPollCreation(msg) {
			t.Errorf("%s: isPollCreation = false", name)
		}
	}
	if PollCreationOf(&waE2E.Message{Conversation: proto.String("hi")}) != nil {
		t.Error("a text message is not a poll")
	}
	if PollCreationOf(nil) != nil {
		t.Error("nil message is not a poll")
	}
}

// A vote on a v6 poll (end time / hidden voter names) carries SHA-256 of the
// chosen option name; it must match the options read from the v6 creation.
func TestVoteHashMatchesV6Options(t *testing.T) {
	msg := &waE2E.Message{PollCreationMessageV6: poll("Beach", "Pool", "Bar")}
	p := PollCreationOf(msg)
	names := make([]string, len(p.GetOptions()))
	for i, o := range p.GetOptions() {
		names[i] = o.GetOptionName()
	}
	vote := whatsmeow.HashPollOptions([]string{"Pool"})[0]
	found := -1
	for i, h := range whatsmeow.HashPollOptions(names) {
		if bytes.Equal(h, vote) {
			found = i
		}
	}
	if found != 1 {
		t.Fatalf("vote hash for %q not matched in v6 options (got index %d)", "Pool", found)
	}
}
