package gows

import (
	"errors"
	"testing"
	"time"

	"github.com/devlikeapro/gows/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

var (
	dfmUser  = types.NewJID("15550100001", types.DefaultUserServer)
	dfmLID   = types.NewJID("111111111111111", types.HiddenUserServer)
	dfmOther = types.NewJID("15550100002", types.DefaultUserServer)
	dfmGroup = types.NewJID("120363000000000001", types.GroupServer)
	dfmTime  = time.Unix(1700000000, 0)
)

func dfmMessage(chat types.JID, id string, fromMe bool, sender types.JID) *types.MessageInfo {
	return &types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:     chat,
			Sender:   sender,
			IsFromMe: fromMe,
			IsGroup:  chat.Server == types.GroupServer,
		},
		ID:        id,
		Timestamp: dfmTime,
	}
}

func TestBuildDeleteMessagesForMe(t *testing.T) {
	sender := types.JID{User: "15550100003", Server: types.DefaultUserServer, Device: 12}
	patch := BuildDeleteMessagesForMe([]*types.MessageInfo{
		dfmMessage(dfmUser, "received", false, dfmUser),
		dfmMessage(dfmUser, "sent", true, dfmOther),
		dfmMessage(dfmGroup, "group-received", false, sender),
		dfmMessage(dfmGroup, "group-sent", true, dfmOther),
	}, true)

	assert.Equal(t, appstate.WAPatchRegularHigh, patch.Type)
	require.Len(t, patch.Mutations, 4)
	want := [][]string{
		{appstate.IndexDeleteMessageForMe, dfmUser.String(), "received", "0", "0"},
		{appstate.IndexDeleteMessageForMe, dfmUser.String(), "sent", "1", "0"},
		// Never the device of the sender, WhatsApp keys messages by user.
		{appstate.IndexDeleteMessageForMe, dfmGroup.String(), "group-received", "0", "15550100003@s.whatsapp.net"},
		{appstate.IndexDeleteMessageForMe, dfmGroup.String(), "group-sent", "1", "0"},
	}
	for i, mutation := range patch.Mutations {
		assert.Equal(t, want[i], mutation.Index)
		assert.Equal(t, int32(3), mutation.Version)
		action := mutation.Value.GetDeleteMessageForMeAction()
		require.NotNil(t, action)
		assert.True(t, action.GetDeleteMedia())
		assert.Equal(t, dfmTime.Unix(), action.GetMessageTimestamp())
	}
}

func TestBuildDeleteMessagesForMeSkipsDuplicates(t *testing.T) {
	patch := BuildDeleteMessagesForMe([]*types.MessageInfo{
		dfmMessage(dfmUser, "id-1", false, dfmUser),
		dfmMessage(dfmUser, "id-1", false, dfmUser),
		dfmMessage(dfmLID, "id-2", false, dfmLID),
	}, false)

	require.Len(t, patch.Mutations, 2)
	assert.Equal(t, "id-1", patch.Mutations[0].Index[2])
	assert.Equal(t, dfmLID.String(), patch.Mutations[1].Index[1])
	assert.False(t, patch.Mutations[0].Value.GetDeleteMessageForMeAction().GetDeleteMedia())
}

func pnForLIDs(pairs map[types.JID]types.JID) func(types.JID) (types.JID, error) {
	return func(lid types.JID) (types.JID, error) {
		return pairs[lid], nil
	}
}

func TestIsSameChat(t *testing.T) {
	known := pnForLIDs(map[types.JID]types.JID{dfmLID: dfmUser})
	cases := []struct {
		name     string
		a, b     types.JID
		pnForLID func(types.JID) (types.JID, error)
		valid    func(lid, pn types.JID) bool
		want     bool
	}{
		{name: "same chat", a: dfmUser, b: dfmUser, want: true},
		{name: "device is ignored", a: types.JID{User: dfmUser.User, Server: dfmUser.Server, Device: 3}, b: dfmUser, want: true},
		{name: "other chat", a: dfmUser, b: dfmOther, pnForLID: known},
		{name: "lid of the phone number", a: dfmLID, b: dfmUser, pnForLID: known, want: true},
		{name: "phone number of the lid", a: dfmUser, b: dfmLID, pnForLID: known, want: true},
		{name: "lid of another phone number", a: dfmLID, b: dfmOther, pnForLID: known},
		{name: "unknown lid", a: dfmLID, b: dfmUser, pnForLID: pnForLIDs(nil)},
		{name: "no lid store", a: dfmLID, b: dfmUser},
		{name: "rejected pair", a: dfmLID, b: dfmUser, pnForLID: known, valid: func(lid, pn types.JID) bool { return false }},
		{name: "group and user", a: dfmGroup, b: dfmUser, pnForLID: known},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := isSameChat(c.a, c.b, c.pnForLID, c.valid)
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}

	_, err := isSameChat(dfmLID, dfmUser, func(types.JID) (types.JID, error) {
		return types.EmptyJID, errors.New("lid store unavailable")
	}, nil)
	assert.Error(t, err)
}

// storedMessagesSpy serves stored messages by id and records the deletes.
type storedMessagesSpy struct {
	*spyMessageStorage
	stored map[types.MessageID]*storage.StoredMessage
}

func (s *storedMessagesSpy) GetMessage(id types.MessageID) (*storage.StoredMessage, error) {
	if msg, ok := s.stored[id]; ok {
		return msg, nil
	}
	return nil, storage.ErrNotFound
}

func newDeleteForMeHandler(stored ...*types.MessageInfo) (*StorageEventHandler, *storedMessagesSpy) {
	st, spy, _, _ := newTestHandler()
	messages := &storedMessagesSpy{spyMessageStorage: spy, stored: map[types.MessageID]*storage.StoredMessage{}}
	for _, info := range stored {
		messages.stored[info.ID] = &storage.StoredMessage{Message: &events.Message{Info: *info}}
	}
	st.storage.Messages = messages
	return st, messages
}

func TestHandleDeleteForMe(t *testing.T) {
	st, messages := newDeleteForMeHandler(dfmMessage(dfmUser, "id-1", false, dfmUser))

	st.processEvent(&events.DeleteForMe{ChatJID: dfmUser, MessageID: "id-1"})

	assert.Equal(t, []types.MessageID{"id-1"}, messages.deleted)
}

func TestHandleDeleteForMeKeepsMessageOfAnotherChat(t *testing.T) {
	st, messages := newDeleteForMeHandler(dfmMessage(dfmOther, "id-1", false, dfmOther))

	st.processEvent(&events.DeleteForMe{ChatJID: dfmUser, MessageID: "id-1"})

	assert.Empty(t, messages.deleted)
}

func TestHandleDeleteForMeUnknownMessage(t *testing.T) {
	st, messages := newDeleteForMeHandler()

	st.processEvent(&events.DeleteForMe{ChatJID: dfmUser, MessageID: "id-1"})

	assert.Empty(t, messages.deleted)
}
