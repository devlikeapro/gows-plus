package main

import (
	"time"

	"github.com/caarlos0/env/v11"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
)

type ClientConfig struct {
	BrowserName string `env:"WAHA_CLIENT_BROWSER_NAME" envDefault:"Firefox"`
	DeviceName  string `env:"WAHA_CLIENT_DEVICE_NAME"  envDefault:"Ubuntu"`
}

func getClientConfig() ClientConfig {
	cfg := ClientConfig{}
	if err := env.Parse(&cfg); err != nil {
		panic(err)
	}
	return cfg
}

// StatusConfig holds environment-variable overrides for status broadcast sending.
type StatusConfig struct {
	// ParticipantsBatchSize controls how many contacts are included per batch
	// when sending a status/story to status@broadcast.
	// Kept large on purpose: what trips WhatsApp's rate limiting is the number of
	// stanzas, not the number of participants inside one. Smaller batches turned
	// the ack timeouts into "server returned error 429" instead of fixing them.
	// Slow acks on a big batch are handled by BatchTimeout below.
	ParticipantsBatchSize int `env:"WAHA_GOWS_STATUS_PARTICIPANTS_BATCH_SIZE" envDefault:"5000"`
	// BatchTimeout bounds how long we wait for the server to acknowledge one
	// batch. whatsmeow defaults to 75s, which is not enough for a large status
	// batch and shows up as "timed out waiting for message send response" even
	// though the batch is delivered.
	// Go duration format: 90s, 180s, 5m.
	BatchTimeout time.Duration `env:"WAHA_GOWS_STATUS_BATCH_TIMEOUT" envDefault:"180s"`
	// BatchDelay is the pause between batches, so a large audience goes out at a
	// steady cadence instead of as a burst of back-to-back stanzas.
	BatchDelay time.Duration `env:"WAHA_GOWS_STATUS_BATCH_DELAY" envDefault:"1500ms"`
	// BatchMaxRetries is how many extra attempts a batch gets after a transient
	// failure (ack timeout or 429). Zero disables retrying.
	BatchMaxRetries int `env:"WAHA_GOWS_STATUS_BATCH_MAX_RETRIES" envDefault:"2"`
	// BatchRetryBackoff is the wait before the first retry; it triples on each
	// further attempt (5s, 15s, ...).
	BatchRetryBackoff time.Duration `env:"WAHA_GOWS_STATUS_BATCH_RETRY_BACKOFF" envDefault:"5s"`
}

func getStatusConfig() StatusConfig {
	cfg := StatusConfig{}
	if err := env.Parse(&cfg); err != nil {
		panic(err)
	}
	return cfg
}

// LinkPreviewConfig holds environment-variable overrides for link preview generation.
type LinkPreviewConfig struct {
	// FetchTimeout bounds fetching the page metadata and the preview image
	// when generating a link preview for an outgoing message.
	// Go duration format: 10s, 30s, 1m.
	FetchTimeout time.Duration `env:"WAHA_GOWS_LINK_PREVIEW_TIMEOUT" envDefault:"10s"`
}

func getLinkPreviewConfig() LinkPreviewConfig {
	cfg := LinkPreviewConfig{}
	if err := env.Parse(&cfg); err != nil {
		panic(err)
	}
	return cfg
}

// KeepAliveConfig overrides whatsmeow's websocket keepalive ping interval.
// Useful behind proxies that reap idle tunnels faster than the default ping,
// which otherwise causes constant "Keepalive timed out" reconnect loops.
type KeepAliveConfig struct {
	// Go durations (8s, 1m); zero (or unset) leaves whatsmeow's default (min 20s / max 30s) in place
	IntervalMin time.Duration `env:"WAHA_GOWS_KEEPALIVE_INTERVAL_MIN"`
	IntervalMax time.Duration `env:"WAHA_GOWS_KEEPALIVE_INTERVAL_MAX"`
}

func getKeepAliveConfig() KeepAliveConfig {
	cfg := KeepAliveConfig{}
	if err := env.Parse(&cfg); err != nil {
		panic(err)
	}
	return cfg
}

// DevicePropsConfig holds optional overrides for waCompanionReg.DeviceProps.
// Each Maybe field has three states:
//
//	Set=false          → env var absent or empty string, field is left unchanged
//	Set=true, Value=nil → env var was "null", proto field set to nil
//	Set=true, Value=v   → env var parsed successfully, proto field set to v
type DevicePropsConfig struct {
	RequireFullSync Maybe[*bool] `env:"WAHA_GOWS_DEVICE_REQUIRE_FULL_SYNC"`

	// DeviceProps_HistorySyncConfig fields
	FullSyncDaysLimit                        Maybe[*uint32] `env:"WAHA_GOWS_DEVICE_HISTORY_SYNC_FULL_SYNC_DAYS_LIMIT"`
	FullSyncSizeMbLimit                      Maybe[*uint32] `env:"WAHA_GOWS_DEVICE_HISTORY_SYNC_FULL_SYNC_SIZE_MB_LIMIT"`
	StorageQuotaMb                           Maybe[*uint32] `env:"WAHA_GOWS_DEVICE_HISTORY_SYNC_STORAGE_QUOTA_MB"`
	InlineInitialPayloadInE2EeMsg            Maybe[*bool]   `env:"WAHA_GOWS_DEVICE_HISTORY_SYNC_INLINE_INITIAL_PAYLOAD_IN_E2EE_MSG"`
	RecentSyncDaysLimit                      Maybe[*uint32] `env:"WAHA_GOWS_DEVICE_HISTORY_SYNC_RECENT_SYNC_DAYS_LIMIT"`
	SupportCallLogHistory                    Maybe[*bool]   `env:"WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_CALL_LOG_HISTORY"`
	SupportBotUserAgentChatHistory           Maybe[*bool]   `env:"WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_BOT_USER_AGENT_CHAT_HISTORY"`
	SupportCagReactionsAndPolls              Maybe[*bool]   `env:"WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_CAG_REACTIONS_AND_POLLS"`
	SupportBizHostedMsg                      Maybe[*bool]   `env:"WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_BIZ_HOSTED_MSG"`
	SupportRecentSyncChunkMessageCountTuning Maybe[*bool]   `env:"WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_RECENT_SYNC_CHUNK_MESSAGE_COUNT_TUNING"`
	SupportHostedGroupMsg                    Maybe[*bool]   `env:"WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_HOSTED_GROUP_MSG"`
	SupportFbidBotChatHistory                Maybe[*bool]   `env:"WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_FBID_BOT_CHAT_HISTORY"`
	SupportAddOnHistorySyncMigration         Maybe[*bool]   `env:"WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_ADD_ON_HISTORY_SYNC_MIGRATION"`
	SupportMessageAssociation                Maybe[*bool]   `env:"WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_MESSAGE_ASSOCIATION"`
	SupportGroupHistory                      Maybe[*bool]   `env:"WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_GROUP_HISTORY"`
	OnDemandReady                            Maybe[*bool]   `env:"WAHA_GOWS_DEVICE_HISTORY_SYNC_ON_DEMAND_READY"`
	SupportGuestChat                         Maybe[*bool]   `env:"WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_GUEST_CHAT"`
	CompleteOnDemandReady                    Maybe[*bool]   `env:"WAHA_GOWS_DEVICE_HISTORY_SYNC_COMPLETE_ON_DEMAND_READY"`
	ThumbnailSyncDaysLimit                   Maybe[*uint32] `env:"WAHA_GOWS_DEVICE_HISTORY_SYNC_THUMBNAIL_SYNC_DAYS_LIMIT"`
	InitialSyncMaxMessagesPerChat            Maybe[*uint32] `env:"WAHA_GOWS_DEVICE_HISTORY_SYNC_INITIAL_SYNC_MAX_MESSAGES_PER_CHAT"`
	SupportManusHistory                      Maybe[*bool]   `env:"WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_MANUS_HISTORY"`
	SupportHatchHistory                      Maybe[*bool]   `env:"WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_HATCH_HISTORY"`
}

// PatchDeviceProps applies DevicePropsConfig overrides onto the current device props.
func PatchDeviceProps(props *waCompanionReg.DeviceProps) {
	cfg := DevicePropsConfig{}
	if err := env.Parse(&cfg); err != nil {
		panic(err)
	}

	if cfg.RequireFullSync.Set {
		props.RequireFullSync = cfg.RequireFullSync.Value
	}

	hasHistory := cfg.FullSyncDaysLimit.Set || cfg.FullSyncSizeMbLimit.Set ||
		cfg.StorageQuotaMb.Set || cfg.InlineInitialPayloadInE2EeMsg.Set ||
		cfg.RecentSyncDaysLimit.Set || cfg.SupportCallLogHistory.Set ||
		cfg.SupportBotUserAgentChatHistory.Set || cfg.SupportCagReactionsAndPolls.Set ||
		cfg.SupportBizHostedMsg.Set || cfg.SupportRecentSyncChunkMessageCountTuning.Set ||
		cfg.SupportHostedGroupMsg.Set || cfg.SupportFbidBotChatHistory.Set ||
		cfg.SupportAddOnHistorySyncMigration.Set || cfg.SupportMessageAssociation.Set ||
		cfg.SupportGroupHistory.Set || cfg.OnDemandReady.Set || cfg.SupportGuestChat.Set ||
		cfg.CompleteOnDemandReady.Set || cfg.ThumbnailSyncDaysLimit.Set ||
		cfg.InitialSyncMaxMessagesPerChat.Set || cfg.SupportManusHistory.Set ||
		cfg.SupportHatchHistory.Set

	if !hasHistory {
		return
	}

	if props.HistorySyncConfig == nil {
		props.HistorySyncConfig = &waCompanionReg.DeviceProps_HistorySyncConfig{}
	}
	h := props.HistorySyncConfig
	if cfg.FullSyncDaysLimit.Set {
		h.FullSyncDaysLimit = cfg.FullSyncDaysLimit.Value
	}
	if cfg.FullSyncSizeMbLimit.Set {
		h.FullSyncSizeMbLimit = cfg.FullSyncSizeMbLimit.Value
	}
	if cfg.StorageQuotaMb.Set {
		h.StorageQuotaMb = cfg.StorageQuotaMb.Value
	}
	if cfg.InlineInitialPayloadInE2EeMsg.Set {
		h.InlineInitialPayloadInE2EeMsg = cfg.InlineInitialPayloadInE2EeMsg.Value
	}
	if cfg.RecentSyncDaysLimit.Set {
		h.RecentSyncDaysLimit = cfg.RecentSyncDaysLimit.Value
	}
	if cfg.SupportCallLogHistory.Set {
		h.SupportCallLogHistory = cfg.SupportCallLogHistory.Value
	}
	if cfg.SupportBotUserAgentChatHistory.Set {
		h.SupportBotUserAgentChatHistory = cfg.SupportBotUserAgentChatHistory.Value
	}
	if cfg.SupportCagReactionsAndPolls.Set {
		h.SupportCagReactionsAndPolls = cfg.SupportCagReactionsAndPolls.Value
	}
	if cfg.SupportBizHostedMsg.Set {
		h.SupportBizHostedMsg = cfg.SupportBizHostedMsg.Value
	}
	if cfg.SupportRecentSyncChunkMessageCountTuning.Set {
		h.SupportRecentSyncChunkMessageCountTuning = cfg.SupportRecentSyncChunkMessageCountTuning.Value
	}
	if cfg.SupportHostedGroupMsg.Set {
		h.SupportHostedGroupMsg = cfg.SupportHostedGroupMsg.Value
	}
	if cfg.SupportFbidBotChatHistory.Set {
		h.SupportFbidBotChatHistory = cfg.SupportFbidBotChatHistory.Value
	}
	if cfg.SupportAddOnHistorySyncMigration.Set {
		h.SupportAddOnHistorySyncMigration = cfg.SupportAddOnHistorySyncMigration.Value
	}
	if cfg.SupportMessageAssociation.Set {
		h.SupportMessageAssociation = cfg.SupportMessageAssociation.Value
	}
	if cfg.SupportGroupHistory.Set {
		h.SupportGroupHistory = cfg.SupportGroupHistory.Value
	}
	if cfg.OnDemandReady.Set {
		h.OnDemandReady = cfg.OnDemandReady.Value
	}
	if cfg.SupportGuestChat.Set {
		h.SupportGuestChat = cfg.SupportGuestChat.Value
	}
	if cfg.CompleteOnDemandReady.Set {
		h.CompleteOnDemandReady = cfg.CompleteOnDemandReady.Value
	}
	if cfg.ThumbnailSyncDaysLimit.Set {
		h.ThumbnailSyncDaysLimit = cfg.ThumbnailSyncDaysLimit.Value
	}
	if cfg.InitialSyncMaxMessagesPerChat.Set {
		h.InitialSyncMaxMessagesPerChat = cfg.InitialSyncMaxMessagesPerChat.Value
	}
	if cfg.SupportManusHistory.Set {
		h.SupportManusHistory = cfg.SupportManusHistory.Value
	}
	if cfg.SupportHatchHistory.Set {
		h.SupportHatchHistory = cfg.SupportHatchHistory.Value
	}
}
