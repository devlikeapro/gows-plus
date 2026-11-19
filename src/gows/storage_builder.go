package gows

import (
	"context"

	"github.com/devlikeapro/gows/storage"
	meowstorage "github.com/devlikeapro/gows/storage/meow"
	"github.com/devlikeapro/gows/storage/noop"
	"github.com/devlikeapro/gows/storage/sqlstorage"
	"github.com/devlikeapro/gows/storage/views"
	"go.mau.fi/whatsmeow/store"
)

// ApplyDeviceStorageConfig swaps disabled whatsmeow device stores for no-ops.
// The NoopStore must keep Error nil: a non-nil error aborts app state sync and status broadcasts,
// while zero values just skip persisting.
func ApplyDeviceStorageConfig(device *store.Device, cfg StorageConfig) {
	if !cfg.Contacts {
		device.Contacts = &store.NoopStore{}
	}
	if !cfg.MessageSecrets {
		device.MsgSecrets = &store.NoopStore{}
	}
	if !device.Initialized {
		device.Container = &DeviceContainerGows{device.Container, cfg}
	}
}

// DeviceContainerGows re-applies the storage config after the first Save (pairing),
// when whatsmeow's initializeDevice overwrites all device stores with the SQL store.
type DeviceContainerGows struct {
	store.DeviceContainer
	cfg StorageConfig
}

func (c *DeviceContainerGows) PutDevice(ctx context.Context, device *store.Device) error {
	err := c.DeviceContainer.PutDevice(ctx, device)
	ApplyDeviceStorageConfig(device, c.cfg)
	return err
}

func BuildStorage(container *sqlstorage.GContainer, gows *GoWS, cfg StorageConfig) *storage.Storage {
	st := &storage.Storage{}
	st.ChatEphemeralSetting = container.NewChatEphemeralSettingStorage()

	if cfg.Contacts {
		st.Contacts = meowstorage.NewContactStorage(gows.Store)
	} else {
		st.Contacts = noop.NewContactStorage()
	}

	if cfg.Messages {
		st.Messages = container.NewMessageStorage()
		st.CallLogs = container.NewCallLogStorage()
	} else {
		st.Messages = noop.NewMessageStorage()
		st.CallLogs = noop.NewCallLogStorage()
	}

	if cfg.Groups {
		st.Groups = container.NewGroupStorage()
		st.Groups = NewGroupCacheStorage(gows, st.Groups, st.ChatEphemeralSetting)
		st.GroupMemberLabels = container.NewGroupMemberLabelStorage()
	} else {
		st.Groups = NewGroupEphemeralOnlyStorage(gows, st.ChatEphemeralSetting)
		st.GroupMemberLabels = noop.NewGroupMemberLabelStorage()
	}

	if cfg.Chats {
		st.Chats = views.NewChatView(st.Messages, st.Contacts, st.Groups)
	} else {
		st.Chats = noop.NewChatStorage()
	}

	if cfg.Labels {
		st.Labels = container.NewLabelStorage()
		st.LabelAssociations = container.NewLabelAssociationStorage()
	} else {
		st.Labels = noop.NewLabelStorage()
		st.LabelAssociations = noop.NewLabelAssociationStorage()
	}

	st.Lidmap = container.NewLidmapStorage()
	return st
}
