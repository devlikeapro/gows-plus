package server

import (
	"context"
	"encoding/json"

	"github.com/devlikeapro/gows/proto"
	"go.mau.fi/whatsmeow/types"
)

// SubmitPasskeyResponse forwards a WebAuthn assertion (produced by an authenticator
// on the web.whatsapp.com origin, e.g. a browser extension) to finish the passkey
// linking step. The assertion is the JSON of whatsmeow's types.WebAuthnResponse.
func (s *Server) SubmitPasskeyResponse(ctx context.Context, req *__.PasskeyResponseRequest) (*__.Empty, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	var resp types.WebAuthnResponse
	if err := json.Unmarshal([]byte(req.GetResponseJson()), &resp); err != nil {
		return nil, err
	}
	if err := cli.SendPasskeyResponse(ctx, &resp); err != nil {
		return nil, err
	}
	return &__.Empty{}, nil
}

// ConfirmPasskey finalizes a passkey pairing after the confirmation code was shown
// to and verified by the user. Only needed when PairPasskeyConfirmation.SkipHandoffUX
// is false; the SkipHandoffUX case is auto-confirmed by whatsmeow's QR channel.
func (s *Server) ConfirmPasskey(ctx context.Context, req *__.Session) (*__.Empty, error) {
	cli, err := s.Sm.Get(req.GetId())
	if err != nil {
		return nil, err
	}
	if err := cli.SendPasskeyConfirmation(ctx); err != nil {
		return nil, err
	}
	return &__.Empty{}, nil
}
