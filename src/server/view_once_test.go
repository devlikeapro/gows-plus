package server

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestAsViewOnceImage(t *testing.T) {
	message, err := asViewOnce(&waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: proto.String("hi")}})
	if err != nil {
		t.Fatal(err)
	}
	image := message.GetViewOnceMessageV2().GetMessage().GetImageMessage()
	if image == nil || !image.GetViewOnce() || image.GetCaption() != "hi" {
		t.Fatalf("expected viewOnceMessageV2 with a view once image, got %v", message)
	}
}

func TestAsViewOnceVideo(t *testing.T) {
	message, err := asViewOnce(&waE2E.Message{VideoMessage: &waE2E.VideoMessage{}})
	if err != nil {
		t.Fatal(err)
	}
	if !message.GetViewOnceMessageV2().GetMessage().GetVideoMessage().GetViewOnce() {
		t.Fatalf("expected viewOnceMessageV2 with a view once video, got %v", message)
	}
}

func TestAsViewOnceVoice(t *testing.T) {
	message, err := asViewOnce(&waE2E.Message{AudioMessage: &waE2E.AudioMessage{PTT: proto.Bool(true)}})
	if err != nil {
		t.Fatal(err)
	}
	audio := message.GetViewOnceMessageV2Extension().GetMessage().GetAudioMessage()
	if audio == nil || !audio.GetViewOnce() || !audio.GetPTT() {
		t.Fatalf("expected viewOnceMessageV2Extension with a view once voice, got %v", message)
	}
}

func TestAsViewOnceRejectsOtherTypes(t *testing.T) {
	_, err := asViewOnce(&waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{}})
	if err == nil {
		t.Fatal("expected an error for a document")
	}
}
