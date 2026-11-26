package media

import (
	"encoding/binary"
	"errors"
)

// WebPInfo describes a WebP image as needed by WhatsApp stickers.
type WebPInfo struct {
	Width    uint32
	Height   uint32
	Animated bool
}

var ErrNotWebP = errors.New("not a WebP image")

// ParseWebP reads the RIFF/WebP header (no decoding) and returns
// the canvas size and whether the image is animated.
func ParseWebP(data []byte) (WebPInfo, error) {
	if len(data) < 30 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return WebPInfo{}, ErrNotWebP
	}
	info := WebPInfo{}
	sized := false
	pos := 12
	for pos+8 <= len(data) {
		fourcc := string(data[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		body := pos + 8
		if size < 0 || body+size > len(data) {
			size = len(data) - body
		}
		chunk := data[body : body+size]
		switch fourcc {
		case "VP8X":
			if len(chunk) >= 10 {
				info.Animated = info.Animated || chunk[0]&0x02 != 0
				info.Width = 1 + (uint32(chunk[4]) | uint32(chunk[5])<<8 | uint32(chunk[6])<<16)
				info.Height = 1 + (uint32(chunk[7]) | uint32(chunk[8])<<8 | uint32(chunk[9])<<16)
				sized = true
			}
		case "ANIM", "ANMF":
			info.Animated = true
		case "VP8 ":
			// Lossy: frame tag (3 bytes) + start code 9d 01 2a + 14-bit width/height
			if !sized && len(chunk) >= 10 && chunk[3] == 0x9d && chunk[4] == 0x01 && chunk[5] == 0x2a {
				info.Width = uint32(binary.LittleEndian.Uint16(chunk[6:8]) & 0x3fff)
				info.Height = uint32(binary.LittleEndian.Uint16(chunk[8:10]) & 0x3fff)
				sized = true
			}
		case "VP8L":
			// Lossless: signature 0x2f + 14-bit (width-1), 14-bit (height-1)
			if !sized && len(chunk) >= 5 && chunk[0] == 0x2f {
				bits := binary.LittleEndian.Uint32(chunk[1:5])
				info.Width = (bits & 0x3fff) + 1
				info.Height = ((bits >> 14) & 0x3fff) + 1
				sized = true
			}
		}
		// Chunks are padded to an even size
		pos = body + size + (size & 1)
	}
	if !sized {
		return WebPInfo{}, ErrNotWebP
	}
	return info, nil
}
