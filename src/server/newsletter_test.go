package server

import (
	"context"
	"errors"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

const testNewsletterJID = "120363144038483540@newsletter"

func failLookupByJID(t *testing.T) func(context.Context, types.JID) (*types.NewsletterMetadata, error) {
	return func(context.Context, types.JID) (*types.NewsletterMetadata, error) {
		t.Fatal("unexpected lookup by JID")
		return nil, nil
	}
}

func failLookupByInvite(t *testing.T) func(context.Context, string) (*types.NewsletterMetadata, error) {
	return func(context.Context, string) (*types.NewsletterMetadata, error) {
		t.Fatal("unexpected lookup by invite code")
		return nil, nil
	}
}

// Before the fix, the error from the JID lookup was assigned to a shadowed err,
// so the caller got a nil response with a nil error and toNewsletter panicked.
func TestNewsletterInfoReturnsJIDLookupError(t *testing.T) {
	lookupErr := errors.New("not authorized")
	byJID := func(_ context.Context, jid types.JID) (*types.NewsletterMetadata, error) {
		if jid.String() != testNewsletterJID {
			t.Fatalf("looked up %s, want %s", jid, testNewsletterJID)
		}
		return nil, lookupErr
	}

	resp, err := newsletterInfo(context.Background(), testNewsletterJID, byJID, failLookupByInvite(t))

	if !errors.Is(err, lookupErr) {
		t.Fatalf("got error %v, want %v", err, lookupErr)
	}
	if resp != nil {
		t.Fatalf("got response %+v, want nil", resp)
	}
}

func TestNewsletterInfoRejectsNilMetadataWithoutError(t *testing.T) {
	byJID := func(context.Context, types.JID) (*types.NewsletterMetadata, error) {
		return nil, nil
	}

	_, err := newsletterInfo(context.Background(), testNewsletterJID, byJID, failLookupByInvite(t))

	if err == nil {
		t.Fatal("expected an error for nil metadata, got nil")
	}
}

func TestNewsletterInfoByJID(t *testing.T) {
	want := &types.NewsletterMetadata{}
	byJID := func(context.Context, types.JID) (*types.NewsletterMetadata, error) {
		return want, nil
	}

	got, err := newsletterInfo(context.Background(), testNewsletterJID, byJID, failLookupByInvite(t))

	if err != nil || got != want {
		t.Fatalf("got (%p, %v), want (%p, nil)", got, err, want)
	}
}

func TestNewsletterInfoByInvite(t *testing.T) {
	const invite = "0029Va4K0PZ5a245NkngBA2M"
	want := &types.NewsletterMetadata{}
	byInvite := func(_ context.Context, key string) (*types.NewsletterMetadata, error) {
		if key != invite {
			t.Fatalf("looked up %s, want %s", key, invite)
		}
		return want, nil
	}

	got, err := newsletterInfo(context.Background(), invite, failLookupByJID(t), byInvite)

	if err != nil || got != want {
		t.Fatalf("got (%p, %v), want (%p, nil)", got, err, want)
	}
}

func TestNewsletterInfoReturnsInviteLookupError(t *testing.T) {
	lookupErr := errors.New("invite not found")
	byInvite := func(context.Context, string) (*types.NewsletterMetadata, error) {
		return nil, lookupErr
	}

	_, err := newsletterInfo(context.Background(), "0029Va4K0PZ5a245NkngBA2M", failLookupByJID(t), byInvite)

	if !errors.Is(err, lookupErr) {
		t.Fatalf("got error %v, want %v", err, lookupErr)
	}
}
