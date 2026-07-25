package server

import (
	"context"

	"github.com/devlikeapro/gows/gows"
	__ "github.com/devlikeapro/gows/proto"
)

// FetchMessageCapping fetches the account's current new-chat message capping
// (per-cycle quota) on demand, for callers that want a fresh value instead of
// the last state pushed through the event stream.
func (s *Server) FetchMessageCapping(ctx context.Context, req *__.Session) (*__.Json, error) {
	cli, err := s.Sm.Get(req.GetId())
	if err != nil {
		return nil, err
	}
	capping, err := cli.FetchMessageCapping(ctx, gows.MessageCappingTypeNewChatThread)
	if err != nil {
		return nil, err
	}
	return toJson(capping)
}
