package signaling

import (
	"testing"

	waBinary "go.mau.fi/whatsmeow/binary"
)

func TestExtractRelayEndpointsTe2Offer(t *testing.T) {
	addr1 := []byte{10, 20, 30, 40, 0x0d, 0x98}
	addr2 := []byte{11, 21, 31, 41, 0x0d, 0x99}

	offer := &waBinary.Node{
		Tag: "offer",
		Attrs: waBinary.Attrs{
			"call-id":      "abc",
			"call-creator": "111@lid",
		},
		Content: []waBinary.Node{
			{
				Tag: "relay",
				Attrs: waBinary.Attrs{
					"uuid":     "relay-uuid",
					"self_pid": "5",
					"peer_pid": "1",
				},
				Content: []waBinary.Node{
					{Tag: "key", Content: []byte("relaykey")},
					{Tag: "token", Attrs: waBinary.Attrs{"id": "0"}, Content: []byte{0xAA}},
					{Tag: "auth_token", Attrs: waBinary.Attrs{"id": "1"}, Content: []byte{0xBB}},
					{
						Tag: "te2",
						Attrs: waBinary.Attrs{
							"token_id": "0", "auth_token_id": "1",
							"relay_name": "fmea2c01", "relay_id": "0", "c2r_rtt": "7",
						},
						Content: addr1,
					},
					{
						Tag: "te2",
						Attrs: waBinary.Attrs{
							"token_id": "0", "auth_token_id": "1",
							"relay_name": "gig4c02", "relay_id": "1", "c2r_rtt": "10",
						},
						Content: addr2,
					},
				},
			},
		},
	}

	relays := ExtractRelayEndpoints(offer)
	if len(relays) != 2 {
		t.Fatalf("expected 2 relays, got %d", len(relays))
	}
	if relays[0].RelayName != "fmea2c01" || relays[0].IP != "10.20.30.40" {
		t.Errorf("relay[0] = %+v", relays[0])
	}
	if relays[1].RelayName != "gig4c02" {
		t.Errorf("relay[1].name = %q", relays[1].RelayName)
	}
	if relays[0].Key != "relaykey" {
		t.Errorf("relay key = %q", relays[0].Key)
	}
}
