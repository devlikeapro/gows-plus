package media

import (
	"os"
	"testing"
)

func TestParseWebP(t *testing.T) {
	cases := []struct {
		file     string
		width    uint32
		height   uint32
		animated bool
	}{
		// VP8X with alpha, single frame
		{"testdata/static.webp", 512, 512, false},
		// VP8X with ANIM/ANMF chunks
		{"testdata/animated.webp", 512, 512, true},
		// Simple lossy (VP8 chunk only)
		{"testdata/lossy.webp", 400, 300, false},
		// Simple lossless (VP8L chunk only)
		{"testdata/lossless.webp", 320, 240, false},
	}
	for _, c := range cases {
		data, err := os.ReadFile(c.file)
		if err != nil {
			t.Fatal(err)
		}
		info, err := ParseWebP(data)
		if err != nil {
			t.Fatalf("%s: %v", c.file, err)
		}
		if info.Animated != c.animated {
			t.Errorf("%s: animated = %v, want %v", c.file, info.Animated, c.animated)
		}
		if info.Width != c.width || info.Height != c.height {
			t.Errorf("%s: size = %dx%d, want %dx%d", c.file, info.Width, info.Height, c.width, c.height)
		}
	}
}

func TestParseWebPRejectsOtherFormats(t *testing.T) {
	inputs := map[string][]byte{
		"png":       []byte("\x89PNG\r\n\x1a\n0000000000000000000000000000"),
		"empty":     {},
		"truncated": []byte("RIFF\x10\x00\x00\x00WEBP"),
	}
	for name, data := range inputs {
		if _, err := ParseWebP(data); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
