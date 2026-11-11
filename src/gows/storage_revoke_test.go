package gows

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikeapro/gows/storage"
	"github.com/devlikeapro/gows/storage/noop"
	"github.com/devlikeapro/gows/storage/sqlstorage"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

var (
	revokeOwnPN      = types.NewJID("15550100001", types.DefaultUserServer)
	revokeOwnLID     = types.NewJID("100000000000001", types.HiddenUserServer)
	revokeAlicePN    = types.NewJID("15550100002", types.DefaultUserServer)
	revokeAliceLID   = types.NewJID("100000000000002", types.HiddenUserServer)
	revokeBobPN      = types.NewJID("15550100003", types.DefaultUserServer)
	revokeAdminPN    = types.NewJID("15550100004", types.DefaultUserServer)
	revokeAdminLID   = types.NewJID("100000000000004", types.HiddenUserServer)
	revokeUnknownLID = types.NewJID("100000000000008", types.HiddenUserServer)
	// Mapped to our own number in the LID store, but it is not our LID
	revokeForeignLID = types.NewJID("100000000000009", types.HiddenUserServer)
	revokeGroup      = types.NewJID("120363000000000001", types.GroupServer)
	revokeChannel    = types.NewJID("120363000000000003", types.NewsletterServer)
	revokeStatus     = types.NewJID("status", types.BroadcastServer)
)

// revokeLIDStore maps LID users to phone number users
type revokeLIDStore map[string]string

func (s revokeLIDStore) PutManyLIDMappings(ctx context.Context, mappings []store.LIDMapping) error {
	return nil
}

func (s revokeLIDStore) PutLIDMapping(ctx context.Context, lid, jid types.JID) error {
	return nil
}

func (s revokeLIDStore) GetPNForLID(ctx context.Context, lid types.JID) (types.JID, error) {
	pn, ok := s[lid.User]
	if !ok {
		return types.EmptyJID, nil
	}
	return types.JID{User: pn, Device: lid.Device, Server: types.DefaultUserServer}, nil
}

func (s revokeLIDStore) GetLIDForPN(ctx context.Context, pn types.JID) (types.JID, error) {
	for lid, user := range s {
		if user == pn.User {
			return types.JID{User: lid, Device: pn.Device, Server: types.HiddenUserServer}, nil
		}
	}
	return types.EmptyJID, nil
}

func (s revokeLIDStore) GetManyLIDsForPNs(ctx context.Context, pns []types.JID) (map[types.JID]types.JID, error) {
	return nil, nil
}

// revokeMessageStorage keeps the stored messages and records the deletes
type revokeMessageStorage struct {
	*noop.MessageStorage
	stored  map[types.MessageID]*storage.StoredMessage
	deleted []types.MessageID
}

func (s *revokeMessageStorage) GetMessage(id types.MessageID) (*storage.StoredMessage, error) {
	msg, ok := s.stored[id]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return msg, nil
}

func (s *revokeMessageStorage) DeleteMessage(id types.MessageID) error {
	s.deleted = append(s.deleted, id)
	return nil
}

type revokeGroupStorage struct {
	*noop.GroupStorage
	groups map[types.JID]*types.GroupInfo
}

func (s *revokeGroupStorage) GetGroup(jid types.JID) (*types.GroupInfo, error) {
	group, ok := s.groups[jid]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return group, nil
}

// revokeTestLog keeps the warnings
type revokeTestLog struct {
	waLog.Logger
	warnings []string
}

func (l *revokeTestLog) Warnf(msg string, args ...any) {
	l.warnings = append(l.warnings, fmt.Sprintf(msg, args...))
}

func revokeDevice(jid types.JID, device uint16) types.JID {
	jid.Device = device
	return jid
}

func newRevokeTestGoWS(lids store.LIDStore) *GoWS {
	own := revokeDevice(revokeOwnPN, 7)
	device := &store.Device{ID: &own, LID: revokeOwnLID, LIDs: lids}
	return &GoWS{Client: &whatsmeow.Client{Store: device}, Context: context.Background()}
}

func newRevokeTestHandler(groups ...*types.GroupInfo) (*StorageEventHandler, *revokeMessageStorage, *revokeTestLog) {
	messages := &revokeMessageStorage{stored: map[types.MessageID]*storage.StoredMessage{}}
	groupStorage := &revokeGroupStorage{GroupStorage: noop.NewGroupStorage(), groups: map[types.JID]*types.GroupInfo{}}
	for _, group := range groups {
		groupStorage.groups[group.JID] = group
	}
	log := &revokeTestLog{Logger: waLog.Noop}
	lids := revokeLIDStore{
		revokeOwnLID.User:     revokeOwnPN.User,
		revokeAliceLID.User:   revokeAlicePN.User,
		revokeAdminLID.User:   revokeAdminPN.User,
		revokeForeignLID.User: revokeOwnPN.User,
	}
	st := &StorageEventHandler{
		gows:    newRevokeTestGoWS(lids),
		log:     log,
		storage: &storage.Storage{Messages: messages, Groups: groupStorage},
	}
	return st, messages, log
}

func revokeEvent(id types.MessageID, source types.MessageSource) *events.Message {
	msg := &waE2E.Message{
		ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_REVOKE.Enum(),
			Key:  &waCommon.MessageKey{ID: proto.String(id)},
		},
	}
	return &events.Message{
		Info:       types.MessageInfo{ID: "REVOKE-" + id, Timestamp: time.Now(), MessageSource: source},
		Message:    msg,
		RawMessage: msg,
	}
}

func revokeGroupWith(participants ...types.GroupParticipant) *types.GroupInfo {
	return &types.GroupInfo{JID: revokeGroup, Participants: participants}
}

func TestRevokeChecksChatAndSender(t *testing.T) {
	inGroup := func(sender types.JID) types.MessageSource {
		return types.MessageSource{Chat: revokeGroup, Sender: sender, IsGroup: true}
	}
	ownInGroup := types.MessageSource{Chat: revokeGroup, Sender: revokeOwnPN, IsFromMe: true, IsGroup: true}
	adminByLID := revokeGroupWith(
		types.GroupParticipant{JID: revokeAlicePN, PhoneNumber: revokeAlicePN},
		types.GroupParticipant{JID: revokeAdminLID, LID: revokeAdminLID, IsAdmin: true},
	)
	adminByPN := revokeGroupWith(
		types.GroupParticipant{JID: revokeAdminPN, PhoneNumber: revokeAdminPN, IsSuperAdmin: true},
	)
	notAdmin := revokeGroupWith(
		types.GroupParticipant{JID: revokeAdminLID, LID: revokeAdminLID, PhoneNumber: revokeAdminPN},
	)

	tests := []struct {
		name    string
		group   *types.GroupInfo
		stored  types.MessageSource
		revoke  types.MessageSource
		deleted bool
	}{
		{
			name:    "author from another device",
			stored:  inGroup(revokeAlicePN),
			revoke:  inGroup(revokeDevice(revokeAlicePN, 3)),
			deleted: true,
		},
		{
			name:    "author with LID, message stored with phone number",
			stored:  inGroup(revokeAlicePN),
			revoke:  inGroup(revokeDevice(revokeAliceLID, 2)),
			deleted: true,
		},
		{
			name:    "author with phone number, message stored with LID",
			stored:  inGroup(revokeAliceLID),
			revoke:  inGroup(revokeAlicePN),
			deleted: true,
		},
		{
			name:   "another member",
			group:  adminByLID,
			stored: inGroup(revokeAlicePN),
			revoke: inGroup(revokeBobPN),
		},
		{
			name:    "group admin",
			group:   adminByLID,
			stored:  inGroup(revokeAlicePN),
			revoke:  inGroup(revokeDevice(revokeAdminLID, 1)),
			deleted: true,
		},
		{
			name:    "group admin stored with phone number",
			group:   adminByPN,
			stored:  inGroup(revokeAlicePN),
			revoke:  inGroup(revokeAdminLID),
			deleted: true,
		},
		{
			name:   "member without admin rights",
			group:  notAdmin,
			stored: inGroup(revokeAlicePN),
			revoke: inGroup(revokeAdminLID),
		},
		{
			name:   "group not stored",
			stored: inGroup(revokeAlicePN),
			revoke: inGroup(revokeAdminLID),
		},
		{
			name:   "author in another chat",
			stored: types.MessageSource{Chat: revokeAlicePN, Sender: revokeAlicePN},
			revoke: inGroup(revokeAlicePN),
		},
		{
			name:    "own revoke from the phone, chat with LID",
			stored:  types.MessageSource{Chat: revokeAlicePN, Sender: revokeOwnPN, IsFromMe: true},
			revoke:  types.MessageSource{Chat: revokeAliceLID, Sender: revokeOwnLID, IsFromMe: true},
			deleted: true,
		},
		{
			name:    "own revoke of a member message",
			stored:  inGroup(revokeAlicePN),
			revoke:  types.MessageSource{Chat: revokeGroup, Sender: revokeDevice(revokeOwnPN, 5), IsFromMe: true, IsGroup: true},
			deleted: true,
		},
		{
			name:   "contact revokes our message",
			stored: types.MessageSource{Chat: revokeAlicePN, Sender: revokeOwnPN, IsFromMe: true},
			revoke: types.MessageSource{Chat: revokeAlicePN, Sender: revokeAlicePN},
		},
		{
			name:   "foreign LID mapped to our number",
			stored: ownInGroup,
			revoke: inGroup(revokeForeignLID),
		},
		{
			name:   "status of another contact",
			stored: types.MessageSource{Chat: revokeStatus, Sender: revokeAlicePN, IsGroup: true},
			revoke: types.MessageSource{Chat: revokeStatus, Sender: revokeBobPN, IsGroup: true},
		},
		{
			name:    "channel",
			stored:  types.MessageSource{Chat: revokeChannel, Sender: revokeOwnPN, IsFromMe: true},
			revoke:  types.MessageSource{Chat: revokeChannel, Sender: revokeChannel},
			deleted: true,
		},
		{
			name:   "direct chat with an unknown LID",
			stored: types.MessageSource{Chat: revokeAlicePN, Sender: revokeAlicePN},
			revoke: types.MessageSource{Chat: revokeUnknownLID, Sender: revokeUnknownLID},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var groups []*types.GroupInfo
			if tt.group != nil {
				groups = append(groups, tt.group)
			}
			st, messages, log := newRevokeTestHandler(groups...)
			id := types.MessageID("MSG-1")
			messages.stored[id] = &storage.StoredMessage{
				Message: &events.Message{Info: types.MessageInfo{ID: id, MessageSource: tt.stored}},
			}

			st.handleMessageEvent(revokeEvent(id, tt.revoke))

			if tt.deleted {
				if len(messages.deleted) != 1 || messages.deleted[0] != id {
					t.Fatalf("expected %v to be deleted, got %v", id, messages.deleted)
				}
				return
			}
			if len(messages.deleted) != 0 {
				t.Fatalf("expected the message to stay, got deleted %v", messages.deleted)
			}
			if len(log.warnings) != 1 {
				t.Fatalf("expected the ignored revoke to be logged, got %v", log.warnings)
			}
		})
	}
}

func TestRevokeOfAMessageNotStored(t *testing.T) {
	st, messages, log := newRevokeTestHandler()

	st.handleMessageEvent(revokeEvent("MSG-UNKNOWN", types.MessageSource{Chat: revokeAlicePN, Sender: revokeAlicePN}))

	if len(messages.deleted) != 0 || len(log.warnings) != 0 {
		t.Fatalf("expected nothing, got deleted %v, warnings %v", messages.deleted, log.warnings)
	}
}

// End to end on a real database: SQLite, or PostgreSQL when GOWS_TEST_PG_DSN is set (run with -p 1)
func TestRevokeOnARealDatabase(t *testing.T) {
	dialect, address := "sqlite3", "file:"+filepath.Join(t.TempDir(), "gows.db")+"?_foreign_keys=on"
	if dsn := os.Getenv("GOWS_TEST_PG_DSN"); dsn != "" {
		dialect, address = "postgres", dsn
	}
	container, err := sqlstorage.New(dialect, address, waLog.Noop)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = container.Close() })
	ctx := context.Background()
	if err = container.LIDMap.PutLIDMapping(ctx, revokeAliceLID, revokeAlicePN); err != nil {
		t.Fatal(err)
	}
	groups := container.NewGroupStorage()
	err = groups.UpsertOneGroup(revokeGroupWith(
		types.GroupParticipant{JID: revokeAdminPN, PhoneNumber: revokeAdminPN, IsAdmin: true},
	))
	if err != nil {
		t.Fatal(err)
	}
	messages := container.NewMessageStorage()
	st := &StorageEventHandler{
		gows:    newRevokeTestGoWS(container.LIDMap),
		log:     waLog.Noop,
		storage: &storage.Storage{Messages: messages, Groups: groups},
	}
	put := func(id types.MessageID, source types.MessageSource) {
		msg := &waE2E.Message{Conversation: proto.String(id)}
		err := messages.UpsertOneMessage(&storage.StoredMessage{
			Message: &events.Message{
				Info:       types.MessageInfo{ID: id, Timestamp: time.Now(), MessageSource: source},
				Message:    msg,
				RawMessage: msg,
			},
			IsReal: true,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	inGroup := func(sender types.JID) types.MessageSource {
		return types.MessageSource{Chat: revokeGroup, Sender: sender, IsGroup: true}
	}
	put("RV-ALICE", inGroup(revokeAlicePN))
	put("RV-BOB", inGroup(revokeBobPN))
	put("RV-DIRECT", types.MessageSource{Chat: revokeAlicePN, Sender: revokeAlicePN})
	put("RV-OWN", types.MessageSource{Chat: revokeAliceLID, Sender: revokeOwnPN, IsFromMe: true})

	st.processEvent(revokeEvent("RV-ALICE", inGroup(revokeBobPN)))
	st.processEvent(revokeEvent("RV-DIRECT", inGroup(revokeAlicePN)))
	for _, id := range []types.MessageID{"RV-ALICE", "RV-DIRECT"} {
		if _, err := messages.GetMessage(id); err != nil {
			t.Fatalf("%s: expected %v to stay, got %v", dialect, id, err)
		}
	}

	st.processEvent(revokeEvent("RV-ALICE", inGroup(revokeDevice(revokeAliceLID, 2))))
	st.processEvent(revokeEvent("RV-BOB", inGroup(revokeDevice(revokeAdminPN, 1))))
	st.processEvent(revokeEvent("RV-OWN", types.MessageSource{Chat: revokeAlicePN, Sender: revokeDevice(revokeOwnPN, 7), IsFromMe: true}))
	for _, id := range []types.MessageID{"RV-ALICE", "RV-BOB", "RV-OWN"} {
		if _, err := messages.GetMessage(id); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("%s: expected %v to be deleted, got %v", dialect, id, err)
		}
	}
}
