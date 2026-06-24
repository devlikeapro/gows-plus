//go:build !mlow

package media

import "errors"

var ErrCodecUnavailable = errors.New("MLow codec unavailable: rebuild with -tags mlow and CGO_ENABLED=1")

func NewMLowCodec(opts CodecOptions) (Codec, error) {
	return nil, ErrCodecUnavailable
}

func NewOpusCodec(sampleRate, frameSize int) (Codec, error) {
	return nil, ErrCodecUnavailable
}
