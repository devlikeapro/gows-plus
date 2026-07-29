package media

import "bytes"

// IsAnimatedWebP reports whether the buffer is an animated WebP
// (RIFF/WEBP with an ANIM chunk). Static WebP files return false.
func IsAnimatedWebP(data []byte) bool {
	if len(data) < 12 {
		return false
	}
	if !bytes.Equal(data[0:4], []byte("RIFF")) {
		return false
	}
	if !bytes.Equal(data[8:12], []byte("WEBP")) {
		return false
	}
	return bytes.Contains(data, []byte("ANIM"))
}

// IsWebP reports whether the buffer looks like a WebP file (RIFF....WEBP).
func IsWebP(data []byte) bool {
	if len(data) < 12 {
		return false
	}
	if !bytes.Equal(data[0:4], []byte("RIFF")) {
		return false
	}
	return bytes.Equal(data[8:12], []byte("WEBP"))
}
