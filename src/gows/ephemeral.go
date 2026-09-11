package gows

import (
	"errors"
	"github.com/devlikeapro/gows/storage"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func (gows *GoWS) PopulateContextInfoDisappearingSettings(info *waE2E.ContextInfo, jid types.JID) (*waE2E.ContextInfo, error) {
	setting, err := gows.getEphemeralSettings(jid)
	if errors.Is(err, storage.ErrNotFound) {
		gows.Log.Debugf("Ephemeral settings not found for %s", jid)
		return info, nil
	}
	if err != nil {
		return info, err
	}
	if setting == nil {
		return info, nil
	}
	if !setting.IsEphemeral {
		return info, nil
	}

	if info == nil {
		info = &waE2E.ContextInfo{}
	}
	info.Expiration = proto.Uint32(setting.Setting.Expiration)

	//
	// NOWEB send only Expiration field, and it works :)
	// But we send all fields for compatibility
	//
	info.EphemeralSettingTimestamp = setting.Setting.Timestamp
	info.DisappearingMode = &waE2E.DisappearingMode{
		Initiator:     setting.Setting.Initiator,
		Trigger:       setting.Setting.Trigger,
		InitiatedByMe: setting.Setting.InitiatedByMe,
	}
	return info, nil
}

func (gows *GoWS) getEphemeralSettings(jid types.JID) (*storage.StoredChatEphemeralSetting, error) {
	if jid.Server == types.GroupServer {
		err := gows.Storage.Groups.FetchGroups(false)
		if err != nil {
			gows.Log.Warnf("Failed fetching groups for ephemeral settings: %v", err)
		}
	}

	switch jid.Server {
	case types.DefaultUserServer, types.HiddenUserServer, types.GroupServer:
		setting, err := gows.Storage.ChatEphemeralSetting.GetChatEphemeralSetting(jid)
		if err != nil {
			return nil, err
		}
		return setting, nil
	}
	return nil, nil
}

// ExtractEphemeralSettingsFromMsg extracts ephemeral settings from a message event (from the initial message).
func ExtractEphemeralSettingsFromMsg(event *events.Message) *storage.StoredChatEphemeralSetting {
	if event.Info.Chat.Server != types.DefaultUserServer && event.Info.Chat.Server != types.HiddenUserServer {
		return nil
	}
	contextInfo := ExtractContextInfo(event.Message)
	if contextInfo == nil {
		return nil
	}
	if contextInfo.Expiration == nil || contextInfo.DisappearingMode == nil {
		return nil
	}

	setting := storage.NotEphemeral(event.Info.Chat)
	setting.Setting = &storage.EphemeralSetting{
		Timestamp:  contextInfo.EphemeralSettingTimestamp,
		Expiration: *contextInfo.Expiration,
	}
	populateFromDisappearingMode(setting.Setting, contextInfo.DisappearingMode)
	setting.IsEphemeral = true
	return setting
}

// ExtractEphemeralSettingsFromProtocolMessage extracts ephemeral settings from a message event.
func ExtractEphemeralSettingsFromProtocolMessage(info types.MessageInfo, protocol *waE2E.ProtocolMessage) *storage.StoredChatEphemeralSetting {
	switch protocol.GetType() {
	case waE2E.ProtocolMessage_EPHEMERAL_SETTING, waE2E.ProtocolMessage_EPHEMERAL_SYNC_RESPONSE:
		var setting *storage.StoredChatEphemeralSetting
		setting = storage.NotEphemeral(info.Chat)
		isEphemeral := protocol.EphemeralExpiration != nil && *protocol.EphemeralExpiration > 0
		if isEphemeral && protocol.DisappearingMode != nil {
			setting.IsEphemeral = true
			timestamp := info.Timestamp.Unix()
			setting.Setting = &storage.EphemeralSetting{
				Timestamp:  &timestamp,
				Expiration: *protocol.EphemeralExpiration,
			}
			populateFromDisappearingMode(setting.Setting, protocol.DisappearingMode)
		}
		return setting
	default:
		return nil
	}
}

func ExtractEphemeralSettingsFromConversation(conv *waHistorySync.Conversation, jid types.JID) *storage.StoredChatEphemeralSetting {
	if conv.EphemeralExpiration == nil || *conv.EphemeralExpiration == 0 {
		return nil
	}
	setting := storage.NotEphemeral(jid)
	setting.IsEphemeral = true
	setting.Setting = &storage.EphemeralSetting{
		Timestamp: conv.EphemeralSettingTimestamp,
	}
	populateFromDisappearingMode(setting.Setting, conv.DisappearingMode)
	return setting
}

func ExtractEphemeralSettingsFromGroup(group *types.GroupInfo) *storage.StoredChatEphemeralSetting {
	if !group.IsEphemeral {
		return storage.NotEphemeral(group.JID)
	}

	setting := &storage.StoredChatEphemeralSetting{
		ID:          group.JID,
		IsEphemeral: true,
		Setting: &storage.EphemeralSetting{
			Initiator:     waE2E.DisappearingMode_CHANGED_IN_CHAT.Enum(),
			Trigger:       waE2E.DisappearingMode_CHAT_SETTING.Enum(),
			InitiatedByMe: proto.Bool(false),
			Expiration:    group.DisappearingTimer,
		},
	}
	return setting
}

func ExtractEphemeralSettingsFromGroupUpdate(update *events.GroupInfo) *storage.StoredChatEphemeralSetting {
	if update == nil || update.Ephemeral == nil {
		return nil
	}
	if !update.Ephemeral.IsEphemeral {
		return storage.NotEphemeral(update.JID)
	}

	setting := &storage.StoredChatEphemeralSetting{
		ID:          update.JID,
		IsEphemeral: true,
		Setting: &storage.EphemeralSetting{
			Initiator:     waE2E.DisappearingMode_CHANGED_IN_CHAT.Enum(),
			Trigger:       waE2E.DisappearingMode_CHAT_SETTING.Enum(),
			InitiatedByMe: proto.Bool(false),
			Expiration:    update.Ephemeral.DisappearingTimer,
		},
	}
	return setting
}

func populateFromDisappearingMode(setting *storage.EphemeralSetting, mode *waE2E.DisappearingMode) {
	if mode == nil {
		return
	}
	if mode.Initiator != nil {
		setting.Initiator = mode.Initiator
	}
	if mode.Trigger != nil {
		setting.Trigger = mode.Trigger
	}
	if mode.InitiatedByMe != nil {
		setting.InitiatedByMe = mode.InitiatedByMe
	}
}
