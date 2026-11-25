package gows

import (
	"strings"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// Batched status stanza id: "<id>__part__<suffix>"
const statusStanzaIDPart = "__part__"

// normalizeStatusStanzaID returns the status id without the batch suffix - the part before "__part__"
func normalizeStatusStanzaID(id types.MessageID) types.MessageID {
	idx := strings.Index(id, statusStanzaIDPart)
	if idx <= 0 {
		return id
	}
	return id[:idx]
}

// normalizeStatusIDs cuts "__part__" from status@broadcast ids in place.
// whatsmeow keeps the stanza id in its own copy for acks and receipts.
func normalizeStatusIDs(event interface{}) {
	switch evt := event.(type) {
	case *events.Message:
		if evt.Info.Chat == types.StatusBroadcastJID {
			evt.Info.ID = normalizeStatusStanzaID(evt.Info.ID)
		}
	case *events.UndecryptableMessage:
		if evt.Info.Chat == types.StatusBroadcastJID {
			evt.Info.ID = normalizeStatusStanzaID(evt.Info.ID)
		}
	case *events.Receipt:
		if evt.Chat != types.StatusBroadcastJID {
			return
		}
		ids := make([]types.MessageID, len(evt.MessageIDs))
		for i, id := range evt.MessageIDs {
			ids[i] = normalizeStatusStanzaID(id)
		}
		evt.MessageIDs = ids
	}
}
