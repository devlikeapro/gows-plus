package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/devlikeapro/gows/proto"
	"go.mau.fi/whatsmeow/types"
)

func (s *Server) RejectCall(ctx context.Context, req *__.RejectCallRequest) (*__.Empty, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	from, err := types.ParseJID(req.GetFrom())
	if err != nil {
		return nil, fmt.Errorf("parse from JID '%s': %w", req.GetFrom(), err)
	}
	if err = cli.RejectVoipCall(ctx, from, req.GetId()); err != nil {
		return nil, err
	}
	return &__.Empty{}, nil
}

func (s *Server) StartCall(ctx context.Context, req *__.StartCallRequest) (*__.StartCallResponse, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jidStr := strings.TrimSpace(req.GetJid())
	if jidStr == "" {
		return nil, fmt.Errorf("jid is required")
	}
	peer, err := types.ParseJID(jidStr)
	if err != nil {
		peer = types.NewJID(normalizePhone(jidStr), types.DefaultUserServer)
	}
	callID, err := cli.StartVoipCall(ctx, peer, req.GetVideo())
	if err != nil {
		return nil, err
	}
	return &__.StartCallResponse{CallId: callID}, nil
}

func (s *Server) AcceptCall(ctx context.Context, req *__.AcceptCallRequest) (*__.Empty, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	ownerID := ""
	if req.OwnerId != nil {
		ownerID = req.GetOwnerId()
	}
	if err = cli.AcceptVoipCall(ctx, req.GetCallId(), ownerID); err != nil {
		return nil, err
	}
	return &__.Empty{}, nil
}

func (s *Server) EndCall(ctx context.Context, req *__.EndCallRequest) (*__.Empty, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	if err = cli.EndVoipCall(ctx, req.GetCallId()); err != nil {
		return nil, err
	}
	return &__.Empty{}, nil
}

func (s *Server) ExchangeCallWebRTC(ctx context.Context, req *__.ExchangeCallWebRTCRequest) (*__.ExchangeCallWebRTCResponse, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.GetSdpOffer()) == "" {
		return nil, fmt.Errorf("sdp_offer is required")
	}
	cm := cli.GetCallState()
	if cm != nil && req.GetCallId() != "" && cm.ID != req.GetCallId() {
		return nil, fmt.Errorf("call_id mismatch: active call is %s", cm.ID)
	}
	answer, err := cli.ExchangeCallWebRTC(req.GetSdpOffer())
	if err != nil {
		return nil, err
	}
	return &__.ExchangeCallWebRTCResponse{SdpAnswer: answer}, nil
}

func (s *Server) GetCallState(ctx context.Context, req *__.Session) (*__.CallStateResponse, error) {
	cli, err := s.Sm.Get(req.GetId())
	if err != nil {
		return nil, err
	}
	state := cli.GetCallState()
	if state == nil {
		return &__.CallStateResponse{Active: false}, nil
	}
	return &__.CallStateResponse{
		Active:    true,
		CallId:    state.ID,
		From:      state.From,
		Direction: state.Direction,
		Status:    state.Status,
		Event:     state.Event,
	}, nil
}

func normalizePhone(phone string) string {
	phone = strings.TrimSpace(phone)
	phone = strings.TrimPrefix(phone, "+")
	return phone
}
