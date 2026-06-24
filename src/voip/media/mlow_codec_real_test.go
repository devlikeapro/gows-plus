//go:build mlow

package media

import (
	"math"
	"testing"
)

func TestMLowCodecRoundtrip(t *testing.T) {
	codec, err := NewMLowCodec(DefaultCodecOptions)
	if err != nil {
		t.Fatalf("NewMLowCodec: %v", err)
	}
	defer codec.Close()

	if codec.FrameSize() != 960 || codec.SampleRate() != 16000 {
		t.Fatalf("unexpected frame=%d rate=%d", codec.FrameSize(), codec.SampleRate())
	}

	frame := make([]float32, 960)
	for i := range frame {
		frame[i] = 0.3 * float32(math.Sin(2*math.Pi*440*float64(i)/16000))
	}

	encoded, err := codec.Encode(frame)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if len(encoded) == 0 {
		t.Fatal("encoded frame is empty")
	}
	t.Logf("encoded %d samples → %d bytes (MLow)", len(frame), len(encoded))

	decoded, err := codec.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(decoded) == 0 {
		t.Fatal("decoded PCM is empty")
	}
	t.Logf("decoded → %d samples", len(decoded))

	plc, err := codec.Decode(nil)
	if err != nil {
		t.Fatalf("Decode(nil) PLC: %v", err)
	}
	if len(plc) == 0 {
		t.Fatal("PLC returned no samples")
	}
}
