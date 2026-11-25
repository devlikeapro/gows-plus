package gows

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func TestNormalizeStatusStanzaID(t *testing.T) {
	cases := map[string]string{
		"AAAA__part__abcdefgh": "AAAA",
		"AAAA":                 "AAAA",
		"__part__abcdefgh":     "__part__abcdefgh",
		"":                     "",
	}
	for in, expected := range cases {
		assert.Equal(t, expected, normalizeStatusStanzaID(in), in)
	}
}

func TestNormalizeStatusIDs(t *testing.T) {
	group := types.NewJID("123456789", types.GroupServer)

	statusMsg := &events.Message{Info: types.MessageInfo{
		MessageSource: types.MessageSource{Chat: types.StatusBroadcastJID},
		ID:            "AAAA__part__abcdefgh",
	}}
	normalizeStatusIDs(statusMsg)
	assert.Equal(t, "AAAA", statusMsg.Info.ID)

	groupMsg := &events.Message{Info: types.MessageInfo{
		MessageSource: types.MessageSource{Chat: group},
		ID:            "AAAA__part__abcdefgh",
	}}
	normalizeStatusIDs(groupMsg)
	assert.Equal(t, "AAAA__part__abcdefgh", groupMsg.Info.ID)

	undecryptable := &events.UndecryptableMessage{Info: types.MessageInfo{
		MessageSource: types.MessageSource{Chat: types.StatusBroadcastJID},
		ID:            "AAAA__part__abcdefgh",
	}}
	normalizeStatusIDs(undecryptable)
	assert.Equal(t, "AAAA", undecryptable.Info.ID)

	original := []types.MessageID{"AAAA__part__abcdefgh", "BBBB"}
	receipt := &events.Receipt{
		MessageSource: types.MessageSource{Chat: types.StatusBroadcastJID},
		MessageIDs:    original,
	}
	normalizeStatusIDs(receipt)
	assert.Equal(t, []types.MessageID{"AAAA", "BBBB"}, receipt.MessageIDs)
	// whatsmeow's slice is not touched
	assert.Equal(t, "AAAA__part__abcdefgh", original[0])

	groupReceipt := &events.Receipt{
		MessageSource: types.MessageSource{Chat: group},
		MessageIDs:    []types.MessageID{"AAAA__part__abcdefgh"},
	}
	normalizeStatusIDs(groupReceipt)
	assert.Equal(t, []types.MessageID{"AAAA__part__abcdefgh"}, groupReceipt.MessageIDs)
}
