package server

import (
	"context"
	"errors"
	"fmt"
	"github.com/devlikeapro/gows/gows"
	waBinary "go.mau.fi/whatsmeow/binary"
	"os"
	"strconv"
	"time"

	"github.com/devlikeapro/gows/media"
	__ "github.com/devlikeapro/gows/proto"
	"github.com/devlikeapro/gows/storage"
	"go.mau.fi/util/random"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func (s *Server) GenerateNewMessageID(ctx context.Context, req *__.Session) (*__.NewMessageIDResponse, error) {
	cli, err := s.Sm.Get(req.GetId())
	if err != nil {
		return nil, err
	}
	id := cli.GenerateMessageID()
	return &__.NewMessageIDResponse{Id: id}, nil
}

func parseParticipantJIDs(participants []string) ([]types.JID, error) {
	jids := make([]types.JID, 0, len(participants))
	for _, p := range participants {
		jid, err := types.ParseJID(p)
		if err != nil {
			return nil, fmt.Errorf("invalid participant jid (%s): %w", p, err)
		}
		jids = append(jids, jid)
	}
	return jids, nil
}

func (s *Server) SendMessage(ctx context.Context, req *__.MessageRequest) (*__.MessageResponse, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jid, err := types.ParseJID(req.GetJid())
	if err != nil {
		return nil, err
	}

	var contextInfo *waE2E.ContextInfo

	requireDisappearingSettings := true
	switch {
	case req.GetPollVote() != nil:
		requireDisappearingSettings = false
	}

	if requireDisappearingSettings {
		contextInfo, err = cli.PopulateContextInfoDisappearingSettings(contextInfo, jid)
		if err != nil {
			cli.Log.Warnf("Failed to get disappearing settings: %v", err)
		}
	}

	if req.ReplyTo != "" {
		contextInfo, err = cli.PopulateContextInfoWithReply(contextInfo, req.ReplyTo)
		if err != nil {
			cli.Log.Warnf("Failed to get message for reply: %v", err)
		}
	}

	if len(req.GetMentions()) > 0 {
		contextInfo = cli.PopulateContextInfoWithMentions(contextInfo, req.GetMentions())
	}

	if req.Media != nil && req.Media.GetContentPath() != "" {
		content, err := os.ReadFile(req.Media.GetContentPath())
		if err != nil {
			cli.Log.Errorf("Failed to read media from '%s': %v", req.Media.GetContentPath(), err)
			return nil, fmt.Errorf("failed to read media from file: %w", err)
		}
		req.Media.Content = content
	}

	var message *waE2E.Message
	extra := whatsmeow.SendRequestExtra{}

	if len(req.Participants) > 0 {
		participants, err := parseParticipantJIDs(req.Participants)
		if err != nil {
			return nil, err
		}
		extra.Participants = participants
	}

	if req.Id != "" {
		extra.ID = req.Id
	}

	if fwd := req.GetForward(); fwd != nil {
		// Forwarding replaces the content: what goes out is the original
		// message, so anything this request says about content is beside the
		// point. The context info built above still applies - it carries the
		// disappearing settings of the chat we are sending to.
		if fwd.GetMessageId() == "" {
			return nil, fmt.Errorf("forward.messageId is required to forward a message")
		}
		stored, err := cli.Storage.Messages.GetMessageWithRetries(fwd.GetMessageId())
		if err != nil {
			// Also the path when the message storage is turned off for this
			// session, in which case there is nothing to forward from.
			return nil, fmt.Errorf("failed to get message to forward '%s': %w", fwd.GetMessageId(), err)
		}
		if stored == nil {
			return nil, fmt.Errorf("message not found: '%s'", fwd.GetMessageId())
		}
		message, err = gows.BuildForwardedMessage(stored.Message, contextInfo, fwd.GetForce())
		if err != nil {
			return nil, fmt.Errorf("failed to forward message '%s': %w", fwd.GetMessageId(), err)
		}
	} else if req.GetPollVote() != nil {
		vote := req.PollVote
		if vote.Options == nil {
			vote.Options = []string{}
		}
		if jid.Server == types.NewsletterServer {
			var serverId int
			if vote.PollServerId == nil {
				stored, err := cli.Storage.Messages.GetMessageWithRetries(vote.PollMessageId)
				if err != nil {
					return nil, fmt.Errorf("failed to get poll creation message %s in %s: %w", vote.PollMessageId, jid, err)
				}
				if stored == nil {
					return nil, fmt.Errorf("message not found: '%s' for '%s'", vote.PollMessageId, jid)
				}
				serverId = stored.Info.ServerID
				if serverId == 0 {
					return nil, fmt.Errorf("server id not found for message: '%s' for '%s', provide PollServerId field explicitly", vote.PollMessageId, jid)
				}
				err = gows.CheckVotesInOptions(stored.Message.Message, vote.Options)
				if err != nil {
					return nil, err
				}
			} else {
				serverId = int(*vote.PollServerId)
			}
			resp, err := cli.SendNewsletterPollVote(ctx, jid, vote.PollMessageId, serverId, vote.Options)
			if err != nil {
				return nil, fmt.Errorf("failed to send poll vote in newsletter: %w", err)
			}
			msg := __.MessageResponse{
				Id:        resp.ID,
				Timestamp: resp.Timestamp.Unix(),
			}
			return &msg, nil
		} else {
			stored, err := cli.Storage.Messages.GetMessageWithRetries(vote.PollMessageId)
			if err != nil {
				return nil, fmt.Errorf("failed to get poll creation message %s in %s: %w", vote.PollMessageId, jid, err)
			}
			if stored == nil {
				return nil, fmt.Errorf("message not found: '%s' for '%s'", vote.PollMessageId, jid)
			}

			err = gows.CheckVotesInOptions(stored.Message.Message, vote.Options)
			if err != nil {
				return nil, err
			}

			// Build poll vote
			message, err = cli.BuildPollVote(ctx, &stored.Info, vote.Options)
			if err != nil {
				return nil, fmt.Errorf("failed to build poll vote message: %w", err)
			}
		}
	} else if req.GetPoll() != nil {
		poll := req.Poll
		selectableOptionCount := 0
		switch poll.MultipleAnswers {
		case false:
			selectableOptionCount = 1
		case true:
			selectableOptionCount = 0
		}
		message = cli.BuildPollCreationV3(poll.Name, poll.Options, selectableOptionCount)
		if message.PollCreationMessageV3 != nil {
			message.PollCreationMessageV3.ContextInfo = contextInfo
		}
		if message.PollCreationMessage != nil {
			message.PollCreationMessage.ContextInfo = contextInfo
		}
		if jid.Server == types.NewsletterServer {
			message.MessageContextInfo = nil
		}
	} else if req.Event != nil {
		var location *gows.EventLocation
		if req.Event.Location != nil {
			location = &gows.EventLocation{
				Name:             req.Event.Location.Name,
				DegreesLongitude: req.Event.Location.DegreesLongitude,
				DegreesLatitude:  req.Event.Location.DegreesLatitude,
			}
		}
		event := &gows.EventMessage{
			Name:               req.Event.Name,
			Description:        req.Event.Description,
			StartTime:          req.Event.StartTime,
			EndTime:            req.Event.EndTime,
			ExtraGuestsAllowed: req.Event.ExtraGuestsAllowed,
			Location:           location,
		}
		message = gows.BuildEventCreation(event)
		message.EventMessage.ContextInfo = contextInfo
	} else if len(req.Contacts) > 0 {
		// Share contacts messages
		contacts := make([]gows.Contact, 0, len(req.Contacts))
		for _, contact := range req.Contacts {
			contacts = append(contacts, gows.Contact{
				DisplayName: contact.DisplayName,
				Vcard:       contact.Vcard,
			})
		}
		message = gows.BuildContactsMessage(contacts, contextInfo)
	} else if req.List != nil {
		// List Message
		listMsg := &waE2E.ListMessage{
			Title:       proto.String(req.List.Title),
			ButtonText:  proto.String(req.List.Button),
			Description: req.List.Description,
			FooterText:  req.List.Footer,
			ListType:    waE2E.ListMessage_SINGLE_SELECT.Enum(),
			Sections:    make([]*waE2E.ListMessage_Section, 0, len(req.List.Sections)),
		}

		// Convert sections
		for _, section := range req.List.Sections {
			waSection := &waE2E.ListMessage_Section{
				Title: proto.String(section.Title),
				Rows:  make([]*waE2E.ListMessage_Row, 0, len(section.Rows)),
			}

			// Convert rows
			for _, row := range section.Rows {
				waRow := &waE2E.ListMessage_Row{
					Title:       proto.String(row.Title),
					RowID:       proto.String(row.RowId),
					Description: row.Description,
				}
				waSection.Rows = append(waSection.Rows, waRow)
			}

			listMsg.Sections = append(listMsg.Sections, waSection)
		}

		listMsg.ContextInfo = contextInfo
		message = &waE2E.Message{
			ListMessage: listMsg,
		}
		node := waBinary.Node{
			Tag: "biz",
			Content: []waBinary.Node{{
				Tag: "list",
				Attrs: waBinary.Attrs{
					"v":    "2",
					"type": "product_list",
				},
			}},
		}
		extra.AdditionalNodes = &[]waBinary.Node{node}

	} else if req.Location != nil {
		// Location Message
		locationMessage := &waE2E.LocationMessage{
			Name:             req.Location.Name,
			DegreesLatitude:  proto.Float64(req.Location.DegreesLatitude),
			DegreesLongitude: proto.Float64(req.Location.DegreesLongitude),
			ContextInfo:      contextInfo,
		}

		message = &waE2E.Message{
			LocationMessage: locationMessage,
		}
	} else if req.Media == nil {
		// Text Message
		message = cli.BuildExtendedTextMessage(req.Text)
		// Link Preview
		if req.LinkPreview {
			var preview *media.LinkPreview
			if req.Preview != nil {
				// Custom preview provided
				preview = &media.LinkPreview{
					Url:         req.Preview.Url,
					Title:       req.Preview.Title,
					Description: req.Preview.Description,
					Image:       req.Preview.Image,
				}
			}
			cli.AddLinkPreviewSafe(jid, message.ExtendedTextMessage, req.LinkPreviewHighQuality, preview)
		}

		message.ExtendedTextMessage.ContextInfo = contextInfo

		// Status Text Message
		var backgroundArgb *uint32
		if req.BackgroundColor != nil {
			backgroundArgb, err = media.ParseColor(req.BackgroundColor.Value)
			if err != nil {
				return nil, err
			}
			message.ExtendedTextMessage.BackgroundArgb = backgroundArgb
		}
		var font *waE2E.ExtendedTextMessage_FontType
		if req.Font != nil {
			font = media.ParseFont(req.Font.Value)
			message.ExtendedTextMessage.Font = font
		}
	} else {
		var mediaResponse whatsmeow.UploadResponse
		message = &waE2E.Message{}
		var mediaType whatsmeow.MediaType
		switch req.Media.Type {
		case __.MediaType_IMAGE:
			// Upload
			mediaType = whatsmeow.MediaImage
			mediaResponse, err = cli.UploadMedia(ctx, jid, req.Media.Content, mediaType)
			if err != nil {
				return nil, err
			}

			// Generate Thumbnail
			thumbnail, err := media.ImageThumbnail(req.Media.Content)
			if err != nil {
				cli.Log.Errorf("Failed to generate thumbnail: %v", err)
			}

			// Get image dimensions
			imgSize, err := media.CurrentSize(req.Media.Content)
			if err != nil {
				cli.Log.Errorf("Failed to get image dimensions: %v", err)
			}

			// Attach
			message.ImageMessage = &waE2E.ImageMessage{
				Caption:       proto.String(req.Text),
				Mimetype:      proto.String(req.Media.Mimetype),
				JPEGThumbnail: thumbnail,
				Height:        proto.Uint32(imgSize.Height),
				Width:         proto.Uint32(imgSize.Width),
				URL:           &mediaResponse.URL,
				DirectPath:    &mediaResponse.DirectPath,
				FileSHA256:    mediaResponse.FileSHA256,
				FileLength:    &mediaResponse.FileLength,
				MediaKey:      mediaResponse.MediaKey,
				FileEncSHA256: mediaResponse.FileEncSHA256,
			}
			message.ImageMessage.ContextInfo = contextInfo
		case __.MediaType_AUDIO:
			mediaType = whatsmeow.MediaAudio
			var waveform []byte
			var duration float32
			// Get waveform and duration if available
			if req.Media.Audio != nil {
				waveform = req.Media.Audio.Waveform
				duration = req.Media.Audio.Duration
			}

			if waveform == nil || len(waveform) == 0 {
				// Generate waveform
				waveform, err = media.Waveform(req.Media.Content)
				if err != nil {
					cli.Log.Errorf("Failed to generate waveform: %v", err)
				}
			}
			if duration == 0 {
				// Get duration
				duration, err = media.Duration(req.Media.Content)
				if err != nil {
					cli.Log.Errorf("Failed to get duration of audio: %v", err)
				}
			}
			durationSeconds := uint32(duration)

			// Upload
			mediaResponse, err = cli.UploadMedia(ctx, jid, req.Media.Content, mediaType)
			if err != nil {
				return nil, err
			}

			// Attach
			ptt := true
			message.AudioMessage = &waE2E.AudioMessage{
				Mimetype:      proto.String(req.Media.Mimetype),
				URL:           &mediaResponse.URL,
				DirectPath:    &mediaResponse.DirectPath,
				MediaKey:      mediaResponse.MediaKey,
				FileEncSHA256: mediaResponse.FileEncSHA256,
				FileSHA256:    mediaResponse.FileSHA256,
				FileLength:    &mediaResponse.FileLength,
				Seconds:       &durationSeconds,
				Waveform:      waveform,
				PTT:           &ptt,
			}
			message.AudioMessage.ContextInfo = contextInfo
		case __.MediaType_VIDEO:
			// NOTE: Keep MediaType_VIDEO and MediaType_PTV in sync
			mediaType = whatsmeow.MediaVideo
			var durationSeconds *uint32
			if req.Media.Video != nil && req.Media.Video.Duration != 0 {
				seconds := uint32(req.Media.Video.Duration)
				durationSeconds = &seconds
			}
			// Upload
			mediaResponse, err = cli.UploadMedia(ctx, jid, req.Media.Content, mediaType)
			if err != nil {
				return nil, err
			}

			// Generate Thumbnail
			thumbnail, err := media.VideoThumbnail(
				req.Media.Content,
				0,
				struct{ Width int }{Width: 72},
			)

			if err != nil {
				cli.Log.Infof("Failed to generate video thumbnail: %v", err)
			}

			var gifPlayback *bool
			var externalShareFullVideoDurationInSeconds *uint32
			if req.Media.Video != nil && req.Media.Video.GifPlayback {
				t := true
				gifPlayback = &t
				zero := uint32(req.Media.Video.ExternalShareFullVideoDurationInSeconds)
				externalShareFullVideoDurationInSeconds = &zero
			}
			message.VideoMessage = &waE2E.VideoMessage{
				Caption:                                 proto.String(req.Text),
				Mimetype:                                proto.String(req.Media.Mimetype),
				URL:                                     &mediaResponse.URL,
				DirectPath:                              &mediaResponse.DirectPath,
				MediaKey:                                mediaResponse.MediaKey,
				FileEncSHA256:                           mediaResponse.FileEncSHA256,
				FileSHA256:                              mediaResponse.FileSHA256,
				FileLength:                              &mediaResponse.FileLength,
				Seconds:                                 durationSeconds,
				JPEGThumbnail:                           thumbnail,
				GifPlayback:                             gifPlayback,
				ExternalShareFullVideoDurationInSeconds: externalShareFullVideoDurationInSeconds,
			}
			message.VideoMessage.ContextInfo = contextInfo
		case __.MediaType_PTV:
			// NOTE: Keep MediaType_VIDEO and MediaType_PTV in sync
			mediaType = whatsmeow.MediaVideo
			var durationSeconds *uint32
			if req.Media.Video != nil && req.Media.Video.Duration != 0 {
				seconds := uint32(req.Media.Video.Duration)
				durationSeconds = &seconds
			}
			// Upload
			mediaResponse, err = cli.UploadMedia(ctx, jid, req.Media.Content, mediaType)
			if err != nil {
				return nil, err
			}

			// Generate Thumbnail
			thumbnail, err := media.VideoThumbnail(
				req.Media.Content,
				0,
				struct{ Width int }{Width: 72},
			)

			if err != nil {
				cli.Log.Infof("Failed to generate ptv thumbnail: %v", err)
			}

			message.PtvMessage = &waE2E.VideoMessage{
				Mimetype:      proto.String(req.Media.Mimetype),
				URL:           &mediaResponse.URL,
				DirectPath:    &mediaResponse.DirectPath,
				MediaKey:      mediaResponse.MediaKey,
				FileEncSHA256: mediaResponse.FileEncSHA256,
				FileSHA256:    mediaResponse.FileSHA256,
				FileLength:    &mediaResponse.FileLength,
				Seconds:       durationSeconds,
				JPEGThumbnail: thumbnail,
			}
			message.PtvMessage.ContextInfo = contextInfo

		case __.MediaType_DOCUMENT:
			mediaType = whatsmeow.MediaDocument
			// Upload
			mediaResponse, err = cli.UploadMedia(ctx, jid, req.Media.Content, mediaType)
			if err != nil {
				return nil, err
			}

			// Generate Thumbnail if possible
			thumbnail, err := media.ImageThumbnail(req.Media.Content)
			if err != nil {
				cli.Log.Infof("Failed to generate thumbnail: %v", err)
			}

			// Attach
			fileName := req.Media.Filename
			if fileName == "" {
				fileName = "Untitled"
			}
			documentMessage := &waE2E.DocumentMessage{
				Caption:       proto.String(req.Text),
				Title:         proto.String(fileName),
				Mimetype:      proto.String(req.Media.Mimetype),
				URL:           proto.String(mediaResponse.URL),
				DirectPath:    proto.String(mediaResponse.DirectPath),
				MediaKey:      mediaResponse.MediaKey,
				FileEncSHA256: mediaResponse.FileEncSHA256,
				FileSHA256:    mediaResponse.FileSHA256,
				FileLength:    proto.Uint64(mediaResponse.FileLength),
				FileName:      proto.String(fileName),
				JPEGThumbnail: thumbnail,
			}

			documentMessage.ContextInfo = contextInfo
			message.DocumentWithCaptionMessage = &waE2E.FutureProofMessage{
				Message: &waE2E.Message{
					DocumentMessage: documentMessage,
				},
			}

		case __.MediaType_STICKER:
			// Stickers upload with the same media keys as images
			mediaType = whatsmeow.MediaImage
			mediaResponse, err = cli.UploadMedia(ctx, jid, req.Media.Content, mediaType)
			if err != nil {
				return nil, err
			}

			// Get sticker dimensions
			imgSize, err := media.CurrentSize(req.Media.Content)
			if err != nil {
				cli.Log.Errorf("Failed to get sticker dimensions: %v", err)
			}

			// Attach
			message.StickerMessage = &waE2E.StickerMessage{
				Mimetype:      proto.String(req.Media.Mimetype),
				Height:        proto.Uint32(imgSize.Height),
				Width:         proto.Uint32(imgSize.Width),
				URL:           &mediaResponse.URL,
				DirectPath:    &mediaResponse.DirectPath,
				FileSHA256:    mediaResponse.FileSHA256,
				FileLength:    &mediaResponse.FileLength,
				MediaKey:      mediaResponse.MediaKey,
				FileEncSHA256: mediaResponse.FileEncSHA256,
			}
			message.StickerMessage.ContextInfo = contextInfo
		}

		// Newsletters
		if mediaResponse.Handle != "" {
			extra.MediaHandle = mediaResponse.Handle
		}
	}

	res, err := cli.SendMessage(ctx, jid, message, extra)
	if err != nil {
		return nil, err
	}
	data, err := toJson(res)
	if err != nil {
		cli.Log.Errorf("Error marshaling message for response %v: %v", res.Info.ID, err)
	}
	msg := __.MessageResponse{
		Id:        res.Info.ID,
		Timestamp: res.Info.Timestamp.Unix(),
		Message:   data,
	}
	return &msg, nil
}

func (s *Server) SendReaction(ctx context.Context, req *__.MessageReaction) (*__.MessageResponse, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jid, err := types.ParseJID(req.Jid)
	if err != nil {
		return nil, err
	}
	if jid.Server == types.NewsletterServer {
		serverID, err := strconv.Atoi(req.MessageId)
		if err != nil {
			cli.Log.Debugf("failed to convert message id (%s) when sending reaction to newsletter: %v", req.MessageId, err)
		}

		if serverID == 0 || err != nil {
			// It's not int - try to get it from storage
			storedMsg, err := cli.Storage.Messages.GetMessageWithRetries(req.MessageId)
			if err != nil {
				cli.Log.Debugf("failed to get message (%s) when sending reaction to newsletter: %v", req.MessageId, err)
			}
			if storedMsg != nil {
				serverID = storedMsg.Message.Info.ServerID
			}
		}
		if serverID == 0 {
			return nil, fmt.Errorf("failed to get server id for newsletter message (%s)", req.MessageId)
		}

		messageID := cli.GenerateMessageID()
		err = cli.NewsletterSendReaction(ctx, jid, types.MessageServerID(serverID), req.Reaction, messageID)
		if err != nil {
			return nil, err
		}
		msg := __.MessageResponse{
			Id:        messageID,
			Timestamp: time.Now().Unix(),
			Message:   nil,
		}
		return &msg, nil
	} else {
		sender, err := types.ParseJID(req.Sender)
		if err != nil {
			return nil, err
		}
		message := cli.BuildReaction(jid, sender, req.MessageId, req.Reaction)
		res, err := cli.SendMessage(ctx, jid, message, whatsmeow.SendRequestExtra{})
		if err != nil {
			return nil, err
		}
		data, err := toJson(res)
		if err != nil {
			cli.Log.Errorf("Error marshaling message for response %v: %v", res.Info.ID, err)
		}
		msg := __.MessageResponse{
			Id:        res.Info.ID,
			Timestamp: res.Info.Timestamp.Unix(),
			Message:   data,
		}
		return &msg, nil
	}
}

func (s *Server) MarkRead(ctx context.Context, req *__.MarkReadRequest) (*__.Empty, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jid, err := types.ParseJID(req.Jid)
	if err != nil {
		return nil, err
	}

	sender, err := types.ParseJID(req.Sender)
	if err != nil {
		return nil, err
	}

	var receiptType types.ReceiptType
	switch req.Type {
	case __.ReceiptType_READ:
		receiptType = types.ReceiptTypeRead
	case __.ReceiptType_PLAYED:
		receiptType = types.ReceiptTypePlayed
	default:
		return nil, errors.New("invalid receipt type: " + req.Type.String())
	}

	ids := req.MessageIds
	if req.MessageId != "" {
		ids = append(ids, req.MessageId)
	}

	if len(ids) == 0 {
		return nil, errors.New("no message ids provided")
	}

	err = cli.MarkRead(ids, jid, sender, receiptType)
	if err != nil {
		return nil, err
	}
	return &__.Empty{}, nil
}

func (s *Server) RevokeMessage(ctx context.Context, req *__.RevokeMessageRequest) (*__.MessageResponse, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jid, err := types.ParseJID(req.Jid)
	if err != nil {
		return nil, err
	}

	var participantJid types.JID
	if req.Sender != "" {
		participantJid, err = types.ParseJID(req.Sender)
		if err != nil {
			return nil, err
		}
	} else {
		participantJid = *cli.Store.ID
	}

	extra := whatsmeow.SendRequestExtra{}
	if len(req.Participants) > 0 {
		participants, err := parseParticipantJIDs(req.Participants)
		if err != nil {
			return nil, err
		}
		extra.Participants = participants
	}

	message := cli.BuildRevoke(jid, participantJid, req.MessageId)
	res, err := cli.SendMessage(ctx, jid, message, extra)
	if err != nil {
		return nil, err
	}

	data, err := toJson(res)
	if err != nil {
		cli.Log.Errorf("Error marshaling message for response %v: %v", res.Info.ID, err)
	}
	msg := __.MessageResponse{
		Id:        res.Info.ID,
		Timestamp: res.Info.Timestamp.Unix(),
		Message:   data,
	}
	return &msg, nil
}

func (s *Server) EditMessage(ctx context.Context, req *__.EditMessageRequest) (*__.MessageResponse, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jid, err := types.ParseJID(req.Jid)
	if err != nil {
		return nil, err
	}

	var originalMessage *waE2E.Message
	storedMsg, err := cli.Storage.Messages.GetMessageWithRetries(req.MessageId)
	if err == nil && storedMsg != nil && storedMsg.Message != nil {
		originalMessage = storedMsg.Message.Message
	} else if err != nil {
		var storageDisabledErr storage.StorageDisabledError
		if !errors.Is(err, storage.ErrNotFound) && !errors.As(err, &storageDisabledErr) {
			cli.Log.Warnf("Failed to fetch original message %s for edit: %v", req.MessageId, err)
		}
	}

	message := cli.BuildEditedMessage(jid, req.Text, originalMessage)
	// Only add link preview if it's an edit of a text message and link preview is requested.
	if req.LinkPreview && message.ExtendedTextMessage != nil {
		cli.AddLinkPreviewSafe(jid, message.ExtendedTextMessage, req.LinkPreviewHighQuality, nil)
	}

	editMessage := cli.BuildEdit(jid, req.MessageId, message)
	res, err := cli.SendMessage(ctx, jid, editMessage, whatsmeow.SendRequestExtra{})
	if err != nil {
		return nil, err
	}

	data, err := toJson(res)
	if err != nil {
		cli.Log.Errorf("Error marshaling message for response %v: %v", res.Info.ID, err)
	}
	msg := __.MessageResponse{
		Id:        res.Info.ID,
		Timestamp: res.Info.Timestamp.Unix(),
		Message:   data,
	}
	return &msg, nil
}

func (s *Server) SendButtonReply(ctx context.Context, req *__.ButtonReplyRequest) (*__.MessageResponse, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jid, err := types.ParseJID(req.GetJid())
	if err != nil {
		return nil, err
	}

	contextInfo, err := cli.PopulateContextInfoWithReply(nil, req.ReplyTo)
	if err != nil {
		cli.Log.Warnf("Failed to get message for reply: %v", err)
	}

	message := &waE2E.Message{
		ButtonsResponseMessage: &waE2E.ButtonsResponseMessage{
			Type: waE2E.ButtonsResponseMessage_DISPLAY_TEXT.Enum(),
			Response: &waE2E.ButtonsResponseMessage_SelectedDisplayText{
				SelectedDisplayText: req.SelectedDisplayText,
			},
			SelectedButtonID: proto.String(req.SelectedButtonID),
			ContextInfo:      contextInfo,
		},
	}

	message.MessageContextInfo = &waE2E.MessageContextInfo{
		MessageSecret: random.Bytes(32),
	}

	res, err := cli.SendMessage(ctx, jid, message, whatsmeow.SendRequestExtra{})
	if err != nil {
		return nil, err
	}
	data, err := toJson(res)
	if err != nil {
		cli.Log.Errorf("Error marshaling message for response %v: %v", res.Info.ID, err)
	}
	msg := __.MessageResponse{
		Id:        res.Info.ID,
		Timestamp: res.Info.Timestamp.Unix(),
		Message:   data,
	}
	return &msg, nil
}

func (s *Server) CancelEventMessage(ctx context.Context, req *__.CancelEventMessageRequest) (*__.MessageResponse, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	jid, err := types.ParseJID(req.Jid)
	if err != nil {
		return nil, err
	}

	eventMessage, err := cli.Storage.Messages.GetMessageWithRetries(req.MessageId)
	if err != nil {
		return nil, err
	}
	if eventMessage == nil {
		return nil, fmt.Errorf("event message not found: %s", req.MessageId)
	}
	update := eventMessage.RawMessage.EventMessage
	update.IsCanceled = proto.Bool(true)
	update.ContextInfo = nil
	message, err := cli.BuildEventUpdate(ctx, &eventMessage.Info, update)

	res, err := cli.SendMessage(ctx, jid, message, whatsmeow.SendRequestExtra{})
	if err != nil {
		return nil, err
	}
	data, err := toJson(res)
	if err != nil {
		cli.Log.Errorf("Error marshaling message for response %v: %v", res.Info.ID, err)
	}
	msg := __.MessageResponse{
		Id:        res.Info.ID,
		Timestamp: res.Info.Timestamp.Unix(),
		Message:   data,
	}
	return &msg, nil
}
