package server

import (
	"context"
	"errors"
	"fmt"
	"github.com/devlikeapro/gows/gows"
	__ "github.com/devlikeapro/gows/proto"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

func toNewsletter(n *types.NewsletterMetadata) *__.Newsletter {
	var picture string
	if n.ThreadMeta.Picture != nil {
		picture = n.ThreadMeta.Picture.URL
		if picture == "" {
			picture = n.ThreadMeta.Picture.DirectPath
		}
	}

	var preview string
	preview = n.ThreadMeta.Preview.URL
	if preview == "" {
		preview = n.ThreadMeta.Preview.DirectPath
	}
	var role string
	if n.ViewerMeta != nil {
		role = string(n.ViewerMeta.Role)
	}
	return &__.Newsletter{
		Id:              n.ID.String(),
		Name:            n.ThreadMeta.Name.Text,
		Description:     n.ThreadMeta.Description.Text,
		Invite:          n.ThreadMeta.InviteCode,
		Picture:         picture,
		Preview:         preview,
		Verified:        n.ThreadMeta.VerificationState == types.NewsletterVerificationStateVerified,
		Role:            role,
		SubscriberCount: int64(n.ThreadMeta.SubscriberCount),
	}
}

func (s *Server) GetSubscribedNewsletters(ctx context.Context, req *__.NewsletterListRequest) (*__.NewsletterList, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	resp, err := cli.GetSubscribedNewsletters(ctx)
	if err != nil {
		return nil, err
	}
	list := make([]*__.Newsletter, len(resp))
	for i, n := range resp {
		list[i] = toNewsletter(n)
	}
	return &__.NewsletterList{Newsletters: list}, nil
}

func (s *Server) GetNewsletterInfo(ctx context.Context, req *__.NewsletterInfoRequest) (result *__.Newsletter, err error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	resp, err := newsletterInfo(ctx, req.GetId(), cli.GetNewsletterInfo, cli.GetNewsletterInfoWithInvite)
	if err != nil {
		return nil, err
	}
	return toNewsletter(resp), nil
}

// newsletterInfo resolves newsletter metadata by "@newsletter" JID or by invite code.
// The lookups are passed in so the error handling can be tested without a WhatsApp connection.
func newsletterInfo(
	ctx context.Context,
	id string,
	byJID func(context.Context, types.JID) (*types.NewsletterMetadata, error),
	byInvite func(context.Context, string) (*types.NewsletterMetadata, error),
) (*types.NewsletterMetadata, error) {
	var resp *types.NewsletterMetadata
	var err error
	if gows.HasNewsletterSuffix(id) {
		var jid types.JID
		jid, err = types.ParseJID(id)
		if err != nil {
			return nil, err
		}
		resp, err = byJID(ctx, jid)
	} else {
		resp, err = byInvite(ctx, id)
	}
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("newsletter %s: no metadata returned", id)
	}
	return resp, nil
}

func (s *Server) CreateNewsletter(ctx context.Context, req *__.CreateNewsletterRequest) (*__.Newsletter, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	params := whatsmeow.CreateNewsletterParams{
		Name:        req.GetName(),
		Description: req.GetDescription(),
		Picture:     req.GetPicture(),
	}
	resp, err := cli.CreateNewsletter(ctx, params)
	if err != nil {
		return nil, err
	}
	return toNewsletter(resp), nil

}

func (s *Server) NewsletterToggleMute(ctx context.Context, req *__.NewsletterToggleMuteRequest) (*__.Empty, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jid, err := types.ParseJID(req.GetJid())
	if err != nil {
		return nil, err
	}
	if !gows.IsNewsletter(jid) {
		return nil, errors.New("invalid jid, not a newsletter")
	}
	err = cli.NewsletterToggleMute(ctx, jid, req.GetMute())
	if err != nil {
		return nil, err
	}
	return &__.Empty{}, nil
}

// NewsletterToggleFollow
func (s *Server) NewsletterToggleFollow(ctx context.Context, req *__.NewsletterToggleFollowRequest) (*__.Empty, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jid, err := types.ParseJID(req.GetJid())
	if err != nil {
		return nil, err
	}
	if !gows.IsNewsletter(jid) {
		return nil, errors.New("invalid jid, not a newsletter")
	}
	if req.Follow {
		err = cli.FollowNewsletter(ctx, jid)
	} else {
		err = cli.UnfollowNewsletter(ctx, jid)
	}
	if err != nil {
		return nil, err
	}
	return &__.Empty{}, nil
}

func (s *Server) GetNewsletterMessagesByInvite(ctx context.Context, req *__.GetNewsletterMessagesByInviteRequest) (*__.Json, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	code := req.Invite
	params := &gows.GetNewsletterMessagesByInviteParams{
		Count: int(req.Limit),
	}
	newsletterMessages, err := cli.GetNewsletterMessagesByInvite(code, params)
	if err != nil {
		return nil, fmt.Errorf("error getting newsletter messages: %w", err)
	}
	response, err := toJson(newsletterMessages)
	if err != nil {
		return nil, fmt.Errorf("error marshaling messages: %w", err)
	}
	return response, nil
}

func (s *Server) SearchNewslettersByView(ctx context.Context, req *__.SearchNewslettersByViewRequest) (*__.NewsletterSearchPageResult, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}

	params := gows.SearchNewsletterByViewParams{
		Page: gows.SearchPageParams{
			Count:       int(req.Page.Limit),
			StartCursor: req.Page.StartCursor,
		},
		View:       req.View,
		Categories: req.Categories,
		Countries:  req.Countries,
	}
	resp, err := cli.SearchNewsletterByView(params)
	if err != nil {
		return nil, fmt.Errorf("error searching newsletters: %w", err)
	}
	result := searchResultToGrpcResponse(resp)
	return result, nil
}

func searchResultToGrpcResponse(resp *gows.SearchNewsletterResult) *__.NewsletterSearchPageResult {
	newsletters := make([]*__.Newsletter, len(resp.Newsletters))
	for i, n := range resp.Newsletters {
		newsletters[i] = toNewsletter(n)
	}
	page := &__.SearchPageResult{
		StartCursor:     resp.Page.StartCursor,
		EndCursor:       resp.Page.EndCursor,
		HasNextPage:     resp.Page.HasNextPage,
		HasPreviousPage: resp.Page.HasPreviousPage,
	}
	result := &__.NewsletterSearchPageResult{
		Newsletters: &__.NewsletterList{Newsletters: newsletters},
		Page:        page,
	}
	return result
}

func (s *Server) SearchNewslettersByText(ctx context.Context, req *__.SearchNewslettersByTextRequest) (*__.NewsletterSearchPageResult, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}

	params := gows.SearchNewsletterByTextParams{
		Page: gows.SearchPageParams{
			Count:       int(req.Page.Limit),
			StartCursor: req.Page.StartCursor,
		},
		Text:       req.Text,
		Categories: req.Categories,
	}
	resp, err := cli.SearchNewsletterByText(params)
	if err != nil {
		return nil, fmt.Errorf("error searching newsletters: %w", err)
	}
	result := searchResultToGrpcResponse(resp)
	return result, nil
}
