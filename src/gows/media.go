package gows

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/devlikeapro/gows/media"
	"github.com/gogo/protobuf/proto"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waMmsRetry"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// mediaDownloader downloads a single downloadable media part. The two
// implementations are whatsmeow's authenticated Download and the anonymous
// signed-path DownloadAnonymous fallback.
type mediaDownloader func(ctx context.Context, msg whatsmeow.DownloadableMessage) ([]byte, error)

func (gows *GoWS) DownloadAnyMedia(ctx context.Context, msg *waE2E.Message) (data []byte, err error) {
	return gows.downloadAnyMedia(ctx, msg, gows.Download)
}

// DownloadAnyMediaAnonymous mirrors DownloadAnyMedia but downloads via the plain
// signed directPath (oh/oe) without the authenticated MMS parameters. Used as a
// fallback on HTTP 403.
func (gows *GoWS) DownloadAnyMediaAnonymous(ctx context.Context, msg *waE2E.Message) (data []byte, err error) {
	return gows.downloadAnyMedia(ctx, msg, gows.DownloadAnonymous)
}

func (gows *GoWS) downloadAnyMedia(ctx context.Context, msg *waE2E.Message, download mediaDownloader) (data []byte, err error) {
	target := unwrapMediaMessage(msg)
	if target == nil {
		return nil, whatsmeow.ErrNothingDownloadableFound
	}
	switch {
	case target.ImageMessage != nil:
		return download(ctx, target.ImageMessage)
	case target.VideoMessage != nil:
		return download(ctx, target.VideoMessage)
	case target.PtvMessage != nil:
		return download(ctx, target.PtvMessage)
	case target.AudioMessage != nil:
		return download(ctx, target.AudioMessage)
	case target.DocumentMessage != nil:
		return download(ctx, target.DocumentMessage)
	case target.DocumentWithCaptionMessage != nil:
		return download(ctx, target.DocumentWithCaptionMessage.Message.DocumentMessage)
	case target.StickerMessage != nil:
		return download(ctx, target.StickerMessage)
	default:
		return nil, whatsmeow.ErrNothingDownloadableFound
	}
}

// DownloadAnyMediaWithRetry wraps DownloadAnyMedia and, on HTTP 403/404/410 or a CDN
// hash mismatch (ErrInvalidMediaEncSHA256), requests the sender's phone to
// re-upload the media via whatsmeow's media-retry protocol.
// On a successful retry the fresh DirectPath is used for a second download attempt.
//
// 404/410 mean the CDN dropped the object (media older than ~3 weeks) - WhatsApp Web
// asks the phone to re-upload in that case too.
// ErrInvalidMediaEncSHA256 ("hash of media ciphertext doesn't match") means the
// CDN object at the message's directPath has been replaced with a different upload
// (e.g. a forwarded image whose original CDN slot was recycled).  The phone
// re-uploads and returns a fresh path — same recovery path as the 403 case.
func (gows *GoWS) DownloadAnyMediaWithRetry(
	ctx context.Context,
	msg *waE2E.Message,
	info *types.MessageInfo,
) ([]byte, error) {
	data, err := gows.DownloadAnyMedia(ctx, msg)
	if err == nil {
		return data, nil
	}

	// On HTTP 403 the authenticated MMS download was rejected even though the
	// message's signed directPath (oh/oe) is frequently still valid. Try an
	// anonymous download of the plain signed path before bothering the phone:
	// this recovers the common "document 403" case instantly, without a
	// media-retry round-trip to the phone (which images/videos never need).
	if errors.Is(err, whatsmeow.ErrMediaDownloadFailedWith403) {
		gows.Log.Infof("Got 403 for '%s', trying anonymous signed-path download", info.ID)
		anonData, anonErr := gows.DownloadAnyMediaAnonymous(ctx, msg)
		if anonErr == nil {
			gows.Log.Infof("Anonymous signed-path download recovered '%s'", info.ID)
			return anonData, nil
		}
		gows.Log.Warnf("Anonymous signed-path download for '%s' failed: %v", info.ID, anonErr)
	}

	retriable := errors.Is(err, whatsmeow.ErrMediaDownloadFailedWith403) ||
		errors.Is(err, whatsmeow.ErrMediaDownloadFailedWith404) ||
		errors.Is(err, whatsmeow.ErrMediaDownloadFailedWith410) ||
		errors.Is(err, whatsmeow.ErrInvalidMediaEncSHA256)
	if !retriable {
		return data, err
	}

	mediaKey := extractMediaKey(msg)
	if len(mediaKey) == 0 {
		gows.Log.Warnf("No media key found for retry of '%s', returning original error: %v", info.ID, err)
		return nil, err
	}

	gows.Log.Infof("Got retriable media error for '%s' (%v), requesting media re-upload from phone", info.ID, err)
	retryEvt, retryErr := gows.requestAndWaitForMediaRetry(ctx, info, mediaKey)
	if retryErr != nil {
		gows.Log.Errorf("Media retry request for '%s' failed: %v", info.ID, retryErr)
		return nil, err
	}

	notification, decryptErr := whatsmeow.DecryptMediaRetryNotification(retryEvt, mediaKey)
	if decryptErr != nil {
		gows.Log.Errorf("Failed to decrypt media retry notification for '%s': %v", info.ID, decryptErr)
		gows.mediaRetryEvents.Delete(info.ID)
		return nil, err
	}
	if notification.GetResult() != waMmsRetry.MediaRetryNotification_SUCCESS {
		gows.Log.Warnf("Media retry for '%s' was not successful: result=%v", info.ID, notification.GetResult())
		gows.mediaRetryEvents.Delete(info.ID)
		return nil, err
	}

	gows.Log.Infof("Got new DirectPath for '%s', retrying download", info.ID)
	updateDirectPath(msg, notification.GetDirectPath())
	data, err = gows.DownloadAnyMedia(ctx, msg)
	if err == nil {
		// Persist the fresh DirectPath so that a later fetch of this message
		// (which reads from storage) returns the recovered media instead of the
		// stale, expired path that originally 403'd.
		gows.persistRefreshedDirectPath(info.ID, notification.GetDirectPath())
	}
	return data, err
}

// persistRefreshedDirectPath writes a DirectPath obtained from a media retry back
// to the stored message, so subsequent re-fetches return the recovered media.
// Best-effort: any failure is logged and swallowed.
func (gows *GoWS) persistRefreshedDirectPath(id types.MessageID, directPath string) {
	if directPath == "" || gows.Storage == nil || gows.Storage.Messages == nil {
		return
	}
	stored, err := gows.Storage.Messages.GetMessage(id)
	if err != nil {
		gows.Log.Warnf("Failed to load message '%s' to persist refreshed DirectPath: %v", id, err)
		return
	}
	if stored == nil || stored.Message == nil {
		return
	}
	updateDirectPath(stored.Message.Message, directPath)
	updateDirectPath(stored.Message.RawMessage, directPath)
	if err := gows.Storage.Messages.UpsertOneMessage(stored); err != nil {
		gows.Log.Warnf("Failed to persist refreshed DirectPath for '%s': %v", id, err)
		return
	}
	gows.Log.Infof("Persisted refreshed DirectPath for '%s'", id)
}

// requestAndWaitForMediaRetry sends a media-retry receipt to the phone (at most once
// per message while a receipt is already in-flight) and waits up to 60 s for the
// *events.MediaRetry response. If a cached event from a previous in-flight attempt
// already exists it is returned immediately without sending another receipt.
func (gows *GoWS) requestAndWaitForMediaRetry(
	ctx context.Context,
	info *types.MessageInfo,
	mediaKey []byte,
) (*events.MediaRetry, error) {
	// Fast path: a previous call already received the event and the TTL cache still holds it.
	if item := gows.mediaRetryEvents.Get(info.ID); item != nil {
		gows.Log.Debugf("Using cached MediaRetry event for '%s'", info.ID)
		return item.Value(), nil
	}

	// Try to register as the sole active waiter.
	// If another goroutine is already waiting, poll the cache instead of
	// sending a duplicate receipt.
	ch := make(chan *events.MediaRetry, 1)
	_, alreadyWaiting := gows.mediaRetryWaiters.LoadOrStore(info.ID, ch)

	if alreadyWaiting {
		gows.Log.Debugf("MediaRetry receipt for '%s' already in-flight, polling for cached event", info.ID)
		retryCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if item := gows.mediaRetryEvents.Get(info.ID); item != nil {
					return item.Value(), nil
				}
			case <-retryCtx.Done():
				return nil, fmt.Errorf("timed out waiting for media retry response for '%s'", info.ID)
			}
		}
	}

	defer gows.mediaRetryWaiters.Delete(info.ID)

	if err := gows.SendMediaRetryReceipt(ctx, info, mediaKey); err != nil {
		return nil, fmt.Errorf("failed to send media retry receipt: %w", err)
	}

	retryCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	select {
	case evt := <-ch:
		return evt, nil
	case <-retryCtx.Done():
		return nil, fmt.Errorf("timed out waiting for media retry response for '%s'", info.ID)
	}
}

// extractMediaKey returns the MediaKey from the downloadable media inside msg.
func extractMediaKey(msg *waE2E.Message) []byte {
	target := unwrapMediaMessage(msg)
	if target == nil {
		return nil
	}
	switch {
	case target.ImageMessage != nil:
		return target.ImageMessage.MediaKey
	case target.VideoMessage != nil:
		return target.VideoMessage.MediaKey
	case target.PtvMessage != nil:
		return target.PtvMessage.MediaKey
	case target.AudioMessage != nil:
		return target.AudioMessage.MediaKey
	case target.DocumentMessage != nil:
		return target.DocumentMessage.MediaKey
	case target.DocumentWithCaptionMessage != nil:
		return target.DocumentWithCaptionMessage.GetMessage().GetDocumentMessage().MediaKey
	case target.StickerMessage != nil:
		return target.StickerMessage.MediaKey
	}
	return nil
}

// updateDirectPath sets a fresh DirectPath on the downloadable media inside msg.
func updateDirectPath(msg *waE2E.Message, path string) {
	target := unwrapMediaMessage(msg)
	if target == nil {
		return
	}
	switch {
	case target.ImageMessage != nil:
		target.ImageMessage.DirectPath = &path
	case target.VideoMessage != nil:
		target.VideoMessage.DirectPath = &path
	case target.PtvMessage != nil:
		target.PtvMessage.DirectPath = &path
	case target.AudioMessage != nil:
		target.AudioMessage.DirectPath = &path
	case target.DocumentMessage != nil:
		target.DocumentMessage.DirectPath = &path
	case target.DocumentWithCaptionMessage != nil:
		if dm := target.DocumentWithCaptionMessage.GetMessage().GetDocumentMessage(); dm != nil {
			dm.DirectPath = &path
		}
	case target.StickerMessage != nil:
		target.StickerMessage.DirectPath = &path
	}
}

func unwrapMediaMessage(msg *waE2E.Message) *waE2E.Message {
	if msg == nil {
		return nil
	}
	if hasMediaPayload(msg) {
		return msg
	}
	nested := []*waE2E.Message{
		getFutureProofMessage(msg.GetEphemeralMessage()),
		getFutureProofMessage(msg.GetViewOnceMessage()),
		getFutureProofMessage(msg.GetViewOnceMessageV2()),
		getFutureProofMessage(msg.GetViewOnceMessageV2Extension()),
		getFutureProofMessage(msg.GetAssociatedChildMessage()),
	}
	for _, child := range nested {
		if child == nil {
			continue
		}
		if resolved := unwrapMediaMessage(child); resolved != nil {
			return resolved
		}
	}
	return nil
}

func getFutureProofMessage(container *waE2E.FutureProofMessage) *waE2E.Message {
	if container == nil {
		return nil
	}
	return container.Message
}

func hasMediaPayload(msg *waE2E.Message) bool {
	if msg == nil {
		return false
	}
	return msg.ImageMessage != nil ||
		msg.VideoMessage != nil ||
		msg.PtvMessage != nil ||
		msg.AudioMessage != nil ||
		msg.DocumentMessage != nil ||
		msg.DocumentWithCaptionMessage != nil ||
		msg.StickerMessage != nil
}

func (gows *GoWS) UploadMedia(
	ctx context.Context,
	jid types.JID,
	content []byte,
	mediaType whatsmeow.MediaType,
) (resp whatsmeow.UploadResponse, err error) {
	if IsNewsletter(jid) {
		resp, err = gows.UploadNewsletter(ctx, content, mediaType)
	} else {
		resp, err = gows.Upload(ctx, content, mediaType)
	}
	return resp, err
}

// AddLinkPreviewSafe adds a link preview to the message if a link is found in the text.
// logs an error if the preview cannot be fetched.
func (gows *GoWS) AddLinkPreviewSafe(jid types.JID, message *waE2E.ExtendedTextMessage, highQuality bool, preview *media.LinkPreview) {
	linkPreviewCtx, cancel := context.WithTimeout(gows.Context, FetchPreviewTimeout)
	defer cancel()
	err := gows.AddLinkPreviewWithContext(linkPreviewCtx, jid, message, highQuality, preview)
	if err != nil {
		gows.Log.Warnf("Failed to add link preview: %v", err)
	}
}

// AddLinkPreviewWithContext adds a link preview to the message if a link is found in the text.
// returns an error if the preview cannot be fetched.
func (gows *GoWS) AddLinkPreviewWithContext(
	ctx context.Context,
	jid types.JID,
	message *waE2E.ExtendedTextMessage,
	highQuality bool,
	preview *media.LinkPreview,
) (err error) {
	var matched string

	if preview == nil {
		// If the preview is nil, we need to extract the URL from the text
		text := message.Text
		matched = media.ExtractUrlFromText(*text)
		if matched == "" {
			return nil
		}
		// "matched" must be exact as it was in the text
		// but scraped URL should be normalized (because it'd also find www.whatsapp.com)
		url := media.MakeSureURL(matched)
		preview, err = media.GoscraperFetchPreview(ctx, url)
		if err != nil || preview == nil {
			return fmt.Errorf("failed to fetch preview info for (%s): %w", url, err)
		}
	} else {
		// If the preview provided, we need to extract the URL from it
		matched = preview.Url
	}

	type_ := waE2E.ExtendedTextMessage_NONE
	message.PreviewType = &type_
	message.MatchedText = &matched
	message.Title = &preview.Title
	message.Description = &preview.Description

	var image []byte
	switch {
	case preview.Image != nil && len(preview.Image) > 0:
		gows.Log.Debugf("Using image data provided from link preview")
		image = preview.Image
	case preview.ImageUrl != "":
		gows.Log.Debugf("Using image URL (%s) from link preview", preview.ImageUrl)
		image, err = media.FetchBodyByUrl(ctx, preview.ImageUrl)
		if err != nil {
			return fmt.Errorf("failed to download image (%s) for link preview: %w", preview.ImageUrl, err)
		}
	case preview.IconUrl != "":
		gows.Log.Debugf("Using icon URL (%s) from link preview", preview.IconUrl)
		image, err = media.FetchBodyByUrl(ctx, preview.IconUrl)
		if err != nil {
			return fmt.Errorf("failed to download icon (%s) for link preview: %w", preview.IconUrl, err)
		}
		highQuality = false
	default:
		gows.Log.Debugf("No image or icon URL found in link preview")
		return nil
	}

	if !highQuality {
		thumbnail, err := media.Resize(image, media.PreviewLinkBuiltInSize)
		if err != nil {
			return fmt.Errorf("failed to generate thumbnail: %w", err)
		}
		message.JPEGThumbnail = thumbnail
	} else {
		thumbnail, err := media.ImageAutoThumbnail(image)
		if err != nil {
			return fmt.Errorf("failed to generate thumbnail: %w", err)
		}
		resp, err := gows.UploadMedia(gows.Context, jid, image, whatsmeow.MediaLinkThumbnail)
		if err != nil {
			return fmt.Errorf("failed to upload image (%s): %w", preview.ImageUrl, err)
		}
		size, err := media.CurrentSize(image)
		if err != nil {
			size = media.PreviewLinkSize
		}
		message.JPEGThumbnail = thumbnail
		message.ThumbnailDirectPath = &resp.DirectPath
		message.ThumbnailSHA256 = resp.FileSHA256
		message.ThumbnailHeight = proto.Uint32(size.Height)
		message.ThumbnailWidth = proto.Uint32(size.Width)
		if !IsNewsletter(jid) {
			// Newsletter thumbnails are uploaded unencrypted (UploadNewsletter),
			// so encryption fields must be absent on the plaintext message
			message.ThumbnailEncSHA256 = resp.FileEncSHA256
			message.MediaKey = resp.MediaKey
			now := time.Now().Unix()
			message.MediaKeyTimestamp = &now
		}
	}
	return nil
}
