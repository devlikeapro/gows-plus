package server

import (
	"context"
	"errors"

	"github.com/devlikeapro/gows/gows"
	__ "github.com/devlikeapro/gows/proto"
	"github.com/devlikeapro/gows/storage"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const maxDeleteMessagesForMe = 100

// DeleteMessagesForMe deletes messages for this account only, nothing is revoked.
// All messages must be stored in the chat (or its LID/phone number) or nothing is sent
func (s *Server) DeleteMessagesForMe(ctx context.Context, req *__.DeleteMessagesForMeRequest) (*__.Empty, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	messages, err := messagesToDeleteForMe(
		req.GetJid(),
		req.GetMessageIds(),
		cli.Storage.Messages.GetMessage,
		func(a, b types.JID) (bool, error) {
			return cli.IsSameChat(ctx, a, b)
		},
	)
	if err != nil {
		return nil, err
	}
	patch := gows.BuildDeleteMessagesForMe(messages, req.GetDeleteMedia())
	if err = cli.SendAppState(ctx, patch); err != nil {
		return nil, err
	}
	return &__.Empty{}, nil
}

// messagesToDeleteForMe looks up the messages and checks they are all in the chat
func messagesToDeleteForMe(
	chat string,
	ids []string,
	getMessage func(id types.MessageID) (*storage.StoredMessage, error),
	isSameChat func(a, b types.JID) (bool, error),
) ([]*types.MessageInfo, error) {
	jid, err := types.ParseJID(chat)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid chat id '%s': %v", chat, err)
	}
	switch jid.Server {
	case types.DefaultUserServer, types.HiddenUserServer, types.GroupServer:
	default:
		return nil, status.Errorf(codes.InvalidArgument, "messages can be deleted for me in direct chats and groups only, not in '%s'", chat)
	}
	if len(ids) == 0 {
		return nil, status.Error(codes.InvalidArgument, "no message ids provided")
	}
	if len(ids) > maxDeleteMessagesForMe {
		return nil, status.Errorf(codes.InvalidArgument, "at most %d messages can be deleted at once", maxDeleteMessagesForMe)
	}

	messages := make([]*types.MessageInfo, 0, len(ids))
	for _, id := range ids {
		stored, err := getMessage(id)
		if errors.Is(err, storage.ErrStorageDisabled) {
			return nil, status.Error(codes.FailedPrecondition, "message storage is disabled for this session, messages to delete for me are looked up there")
		}
		if errors.Is(err, storage.ErrNotFound) || (err == nil && (stored == nil || stored.Message == nil)) {
			return nil, status.Errorf(codes.NotFound, "message not found: '%s'", id)
		}
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to get message '%s': %v", id, err)
		}
		same, err := isSameChat(stored.Info.Chat, jid)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to check the chat of message '%s': %v", id, err)
		}
		if !same {
			return nil, status.Errorf(codes.InvalidArgument, "message '%s' is not in chat '%s'", id, chat)
		}
		messages = append(messages, &stored.Info)
	}
	return messages, nil
}
