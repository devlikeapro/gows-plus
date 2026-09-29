package sqlstorage

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/devlikeapro/gows/storage"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

const pgDSNEnv = "GOWS_TEST_PG_DSN"

const testText = "hello"

var testGroup = types.NewJID("120363412795062969", types.GroupServer)

func TestUpsertMessageKeepsContentOverSenderKeyOnly(t *testing.T) {
	cases := []struct {
		name     string
		saves    []bool
		wantReal bool
	}{
		{"content then sender key", []bool{true, false}, true},
		{"sender key then content", []bool{false, true}, true},
		{"sender key twice", []bool{false, false}, false},
	}
	forEachDialect(t, func(t *testing.T, store *SqlMessageStore) {
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				id := strconv.FormatInt(time.Now().UnixNano(), 16)
				for _, isReal := range c.saves {
					if err := store.UpsertOneMessage(storedMessage(id, isReal)); err != nil {
						t.Fatal(err)
					}
				}

				var isReal bool
				if err := store.db.Get(&isReal, "SELECT is_real FROM gows_messages WHERE id = $1", id); err != nil {
					t.Fatal(err)
				}
				if isReal != c.wantReal {
					t.Fatalf("is_real = %v, want %v", isReal, c.wantReal)
				}

				msg, err := store.GetMessage(id)
				if err != nil {
					t.Fatal(err)
				}
				if c.wantReal && msg.Message.Message.GetConversation() != testText {
					t.Fatalf("content lost: %v", msg.Message.Message)
				}
				if !c.wantReal && msg.Message.Message.GetSenderKeyDistributionMessage() == nil {
					t.Fatalf("sender key lost: %v", msg.Message.Message)
				}
			})
		}
	})
}

func forEachDialect(t *testing.T, test func(t *testing.T, store *SqlMessageStore)) {
	t.Run("sqlite", func(t *testing.T) {
		address := "file:" + filepath.Join(t.TempDir(), "gows.db") + "?_foreign_keys=on"
		test(t, openMessageStore(t, "sqlite3", address))
	})
	t.Run("postgres", func(t *testing.T) {
		dsn := os.Getenv(pgDSNEnv)
		if dsn == "" {
			t.Skipf("%s is not set", pgDSNEnv)
		}
		test(t, openMessageStore(t, "postgres", dsn))
	})
}

func openMessageStore(t *testing.T, dialect, address string) *SqlMessageStore {
	container, err := New(dialect, address, waLog.Noop)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = container.Close() })
	return container.NewMessageStorage()
}

func storedMessage(id string, isReal bool) *storage.StoredMessage {
	message := &waE2E.Message{
		SenderKeyDistributionMessage: &waE2E.SenderKeyDistributionMessage{GroupID: proto.String(testGroup.String())},
	}
	if isReal {
		message = &waE2E.Message{Conversation: proto.String(testText)}
	}
	return &storage.StoredMessage{
		Message: &events.Message{
			Info: types.MessageInfo{
				MessageSource: types.MessageSource{Chat: testGroup},
				ID:            id,
				Timestamp:     time.Now(),
			},
			Message: message,
		},
		IsReal: isReal,
	}
}
