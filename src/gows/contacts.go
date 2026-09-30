package gows

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func (gows *GoWS) UpdateContact(ctx context.Context, jid types.JID, firstname, lastname string) error {
	lid, err := gows.ResolveLidByPN(ctx, jid)
	if err != nil {
		return fmt.Errorf("error resolving lid for %v: %w", jid, err)
	}
	patch := BuildContactUpdate(jid, lid, firstname, lastname)
	err = gows.SendAppState(ctx, patch)
	if err != nil {
		return fmt.Errorf("error updating contact: %w", err)
	}

	mutation := patch.Mutations[0]
	act := mutation.Value.ContactAction
	var ts time.Time

	if mutation.Value.Timestamp != nil {
		ts = time.Unix(*mutation.Value.Timestamp, 0)
	} else {
		ts = time.Now()
	}

	evt := &events.Contact{JID: jid, Timestamp: ts, Action: act, FromFullSync: false}
	go gows.handleEvent(evt)
	return nil
}
