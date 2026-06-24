package call

import (
	"strings"
	"github.com/devlikeapro/gows/voip/core"
	"github.com/devlikeapro/gows/voip/wanode"

	waBinary "go.mau.fi/whatsmeow/binary"
)

func hasChildTag(n *waBinary.Node, tag string) bool {
	for _, c := range wanode.NodeChildren(n) {
		if c.Tag == tag {
			return true
		}
	}
	return false
}

func ensureDeviceJid(jid string) string {
	if strings.Contains(jid, ":") {

		if at := strings.Index(jid, "@"); at > strings.Index(jid, ":") {
			return jid
		}
	}
	return strings.Replace(jid, "@", ":0@", 1)
}

func findOurDevice(participants []string, ourBase, fallback string) string {
	for _, jid := range participants {
		if wanode.CleanJID(jid) == ourBase && strings.Contains(jid, ":") {
			return jid
		}
	}
	return fallback
}

func firstPeerDevice(participants []string, ourBase string) string {
	for _, jid := range participants {
		if wanode.CleanJID(jid) != ourBase {
			return jid
		}
	}
	return ""
}

func firstSsrc(s []uint32) uint32 {
	if len(s) > 0 {
		return s[0]
	}
	return 0
}

func relayEndpointCount(rd *core.RelayData) int {
	if rd == nil {
		return 0
	}
	return len(rd.Endpoints)
}

func containsSsrc(s []uint32, v uint32) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
