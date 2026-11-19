package server

import (
	"context"
	"fmt"

	"github.com/devlikeapro/gows/proto"
	"github.com/devlikeapro/gows/storage"
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
	if err = cli.RejectCall(ctx, from, req.GetId()); err != nil {
		return nil, err
	}
	return &__.Empty{}, nil
}

func (s *Server) GetCallLogs(ctx context.Context, req *__.GetCallLogsRequest) (*__.JsonList, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	filter := storage.CallLogFilter{}
	if req.GetJid() != "" {
		jid, err := types.ParseJID(req.GetJid())
		if err != nil {
			return nil, fmt.Errorf("parse jid '%s': %w", req.GetJid(), err)
		}
		filter.Jid = &jid
	}
	pagination := storage.Pagination{
		Limit:  req.GetPagination().GetLimit(),
		Offset: req.GetPagination().GetOffset(),
	}
	logs, err := cli.Storage.CallLogs.GetCallLogs(filter, pagination)
	if err != nil {
		return nil, err
	}
	return toJsonList(logs)
}
