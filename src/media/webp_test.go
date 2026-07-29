package media

import "testing"

func TestIsWebP(t *testing.T) {
	valid := []byte("RIFF\x00\x00\x00\x00WEBPXXXX")
	if !IsWebP(valid) {
		t.Fatal("expected valid WebP header to be detected")
	}
	if IsWebP([]byte("not a webp")) {
		t.Fatal("expected non-WebP buffer to be rejected")
	}
}

func TestIsAnimatedWebP(t *testing.T) {
	static := []byte("RIFF\x00\x00\x00\x00WEBPxxxx")
	if IsAnimatedWebP(static) {
		t.Fatal("static WebP should not be animated")
	}
	animated := []byte("RIFF\x00\x00\x00\x00WEBPxxxxANIMyyyy")
	if !IsAnimatedWebP(animated) {
		t.Fatal("WebP with ANIM chunk should be animated")
	}
}
