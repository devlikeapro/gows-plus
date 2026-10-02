package server

import (
	"context"
	"fmt"
	"github.com/devlikeapro/gows/media"
	__ "github.com/devlikeapro/gows/proto"
	"github.com/devlikeapro/gows/storage"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

func (s *Server) GetGroups(ctx context.Context, req *__.Session) (*__.JsonList, error) {
	cli, err := s.Sm.Get(req.GetId())
	if err != nil {
		return nil, err
	}
	sort := storage.Sort{Field: "id", Order: storage.SortAsc}
	groups, err := cli.Storage.Groups.GetAllGroups(sort, storage.Pagination{})
	if err != nil {
		return nil, err
	}
	return toJsonList(groups)
}

func (s *Server) FetchGroups(ctx context.Context, req *__.Session) (*__.Empty, error) {
	cli, err := s.Sm.Get(req.GetId())
	if err != nil {
		return nil, err
	}
	err = cli.Storage.Groups.FetchGroups(true)
	if err != nil {
		return nil, err
	}
	return &__.Empty{}, nil
}

func (s *Server) GetGroupInfo(ctx context.Context, req *__.JidRequest) (*__.Json, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jid, err := types.ParseJID(req.GetJid())
	if err != nil {
		return nil, err
	}
	info, err := cli.Storage.Groups.GetGroup(jid)
	if err != nil {
		return nil, err
	}
	return toJson(info)
}

func (s *Server) CreateGroup(ctx context.Context, req *__.CreateGroupRequest) (*__.Json, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jids := make([]types.JID, 0, len(req.Participants))
	for ind, j := range req.Participants {
		jid, err := types.ParseJID(j)
		if err != nil {
			return nil, fmt.Errorf("failed to parse JID at index %d (%s): %w", ind, j, err)
		}
		jids = append(jids, jid)
	}

	request := whatsmeow.ReqCreateGroup{
		Name:         req.Name,
		Participants: jids,
	}
	group, err := cli.CreateGroup(ctx, request)
	if err != nil {
		return nil, err
	}
	return toJson(group)
}

func (s *Server) LeaveGroup(ctx context.Context, req *__.JidRequest) (*__.Empty, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jid, err := types.ParseJID(req.GetJid())
	if err != nil {
		return nil, err
	}
	err = cli.LeaveGroup(ctx, jid)
	if err != nil {
		return nil, err
	}
	return &__.Empty{}, nil
}

func (s *Server) GetGroupInviteLink(ctx context.Context, req *__.JidRequest) (*__.OptionalString, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jid, err := types.ParseJID(req.GetJid())
	if err != nil {
		return nil, err
	}
	link, err := cli.GetGroupInviteLink(ctx, jid, false)
	if err != nil {
		return nil, err
	}
	return &__.OptionalString{Value: link}, nil
}

func (s *Server) RevokeGroupInviteLink(ctx context.Context, req *__.JidRequest) (*__.OptionalString, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jid, err := types.ParseJID(req.GetJid())
	if err != nil {
		return nil, err
	}
	link, err := cli.GetGroupInviteLink(ctx, jid, false)
	if err != nil {
		return nil, err
	}
	return &__.OptionalString{Value: link}, nil
}

func (s *Server) GetGroupInfoFromLink(ctx context.Context, req *__.GroupCodeRequest) (*__.Json, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	info, err := cli.GetGroupInfoFromLink(ctx, req.GetCode())
	if err != nil {
		return nil, err
	}
	return toJson(info)
}

func (s *Server) JoinGroupWithLink(ctx context.Context, req *__.GroupCodeRequest) (*__.Json, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jid, err := cli.JoinGroupWithLink(ctx, req.GetCode())
	if err != nil {
		return nil, err
	}
	resp := make(map[string]interface{})
	resp["jid"] = jid
	return toJson(resp)
}

func (s *Server) SetGroupName(ctx context.Context, req *__.JidStringRequest) (*__.Empty, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jid, err := types.ParseJID(req.GetJid())
	if err != nil {
		return nil, err
	}
	err = cli.SetGroupName(ctx, jid, req.GetValue())
	if err != nil {
		return nil, err
	}
	return &__.Empty{}, nil
}

func (s *Server) SetGroupDescription(ctx context.Context, req *__.JidStringRequest) (*__.Empty, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jid, err := types.ParseJID(req.GetJid())
	if err != nil {
		return nil, err
	}
	prevTopicId := ""
	group, err := cli.GetGroupInfo(ctx, jid)
	if err != nil {
		return nil, fmt.Errorf("failed to get group info: %w", err)
	}
	prevTopicId = group.TopicID
	err = cli.SetGroupDescription(ctx, jid, req.GetValue(), prevTopicId)
	if err != nil {
		return nil, err
	}
	return &__.Empty{}, nil
}

func (s *Server) SetGroupPicture(ctx context.Context, req *__.SetPictureRequest) (*__.Empty, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jid, err := types.ParseJID(req.GetJid())
	if err != nil {
		return nil, err
	}
	content := req.Picture
	var picture []byte
	if len(content) != 0 {
		picture, err = media.ProfilePicture(req.Picture)
		if err != nil {
			return nil, err
		}
	}
	_, err = cli.SetGroupPhoto(ctx, jid, picture)
	if err != nil {
		return nil, err
	}
	return &__.Empty{}, nil
}

func (s *Server) SetGroupLocked(ctx context.Context, req *__.JidBoolRequest) (*__.Empty, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jid, err := types.ParseJID(req.GetJid())
	if err != nil {
		return nil, err
	}
	err = cli.SetGroupLocked(ctx, jid, req.GetValue())
	if err != nil {
		return nil, err
	}
	return &__.Empty{}, nil
}

func (s *Server) SetGroupAnnounce(ctx context.Context, req *__.JidBoolRequest) (*__.Empty, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jid, err := types.ParseJID(req.GetJid())
	if err != nil {
		return nil, err
	}
	err = cli.SetGroupAnnounce(ctx, jid, req.GetValue())
	if err != nil {
		return nil, err
	}
	return &__.Empty{}, nil
}

func (s *Server) SetGroupMemberAddMode(ctx context.Context, req *__.JidBoolRequest) (*__.Empty, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jid, err := types.ParseJID(req.GetJid())
	if err != nil {
		return nil, err
	}
	mode := types.GroupMemberAddModeAdmin
	if req.GetValue() {
		mode = types.GroupMemberAddModeAllMember
	}
	err = cli.SetGroupMemberAddMode(ctx, jid, mode)
	if err != nil {
		return nil, err
	}
	return &__.Empty{}, nil
}

func (s *Server) SetGroupMemberShareHistoryMode(ctx context.Context, req *__.JidBoolRequest) (*__.Empty, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jid, err := types.ParseJID(req.GetJid())
	if err != nil {
		return nil, err
	}
	mode := types.GroupMemberShareHistoryModeAdmin
	if req.GetValue() {
		mode = types.GroupMemberShareHistoryModeAllMember
	}
	err = cli.SetGroupMemberShareHistoryMode(ctx, jid, mode)
	if err != nil {
		return nil, err
	}
	// No notification is parsed for this property, keep the cached group in sync
	group, err := cli.Storage.Groups.GetGroup(jid)
	if err == nil && group != nil {
		group.MemberShareHistoryMode = mode
		_ = cli.Storage.Groups.UpsertOneGroup(group)
	}
	return &__.Empty{}, nil
}

func (s *Server) UpdateGroupParticipants(ctx context.Context, req *__.UpdateParticipantsRequest) (*__.JsonList, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jid, err := types.ParseJID(req.GetJid())
	if err != nil {
		return nil, err
	}
	participants := make([]types.JID, 0, len(req.GetParticipants()))
	for ind, p := range req.GetParticipants() {
		jid, err := types.ParseJID(p)
		if err != nil {
			return nil, fmt.Errorf("failed to parse JID at index %d (%s): %w", ind, p, err)
		}
		participants = append(participants, jid)
	}

	var action whatsmeow.ParticipantChange
	switch req.Action {
	case __.ParticipantAction_ADD:
		action = whatsmeow.ParticipantChangeAdd
	case __.ParticipantAction_REMOVE:
		action = whatsmeow.ParticipantChangeRemove
	case __.ParticipantAction_PROMOTE:
		action = whatsmeow.ParticipantChangePromote
	case __.ParticipantAction_DEMOTE:
		action = whatsmeow.ParticipantChangeDemote
	default:
		return nil, fmt.Errorf("unknown action: %v", req.Action)
	}
	result, err := cli.UpdateGroupParticipants(ctx, jid, participants, action)
	if err != nil {
		return nil, err
	}
	return toJsonList(result)
}

func (s *Server) SetGroupJoinApprovalMode(ctx context.Context, req *__.JidBoolRequest) (*__.Empty, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jid, err := types.ParseJID(req.GetJid())
	if err != nil {
		return nil, err
	}
	err = cli.SetGroupJoinApprovalMode(ctx, jid, req.GetValue())
	if err != nil {
		return nil, err
	}
	return &__.Empty{}, nil
}

func (s *Server) GetGroupRequestParticipants(ctx context.Context, req *__.JidRequest) (*__.JsonList, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jid, err := types.ParseJID(req.GetJid())
	if err != nil {
		return nil, err
	}
	result, err := cli.GetGroupRequestParticipants(ctx, jid)
	if err != nil {
		return nil, err
	}
	return toJsonList(result)
}

func (s *Server) UpdateGroupRequestParticipants(ctx context.Context, req *__.UpdateRequestParticipantsRequest) (*__.JsonList, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jid, err := types.ParseJID(req.GetJid())
	if err != nil {
		return nil, err
	}
	participants := make([]types.JID, 0, len(req.GetParticipants()))
	for ind, p := range req.GetParticipants() {
		jid, err := types.ParseJID(p)
		if err != nil {
			return nil, fmt.Errorf("failed to parse JID at index %d (%s): %w", ind, p, err)
		}
		participants = append(participants, jid)
	}

	var action whatsmeow.ParticipantRequestChange
	switch req.Action {
	case __.ParticipantRequestAction_APPROVE:
		action = whatsmeow.ParticipantChangeApprove
	case __.ParticipantRequestAction_REJECT:
		action = whatsmeow.ParticipantChangeReject
	default:
		return nil, fmt.Errorf("unknown action: %v", req.Action)
	}
	result, err := cli.UpdateGroupRequestParticipants(ctx, jid, participants, action)
	if err != nil {
		return nil, err
	}
	return toJsonList(result)
}
