# Environment Variables

## Client Identity

Variables that control how the GOWS client identifies itself to WhatsApp.

```bash
# Browser name reported to WhatsApp
# Values: Chrome, Firefox, IE, Opera, Safari, Edge
# Default: Firefox
WAHA_CLIENT_BROWSER_NAME=Firefox

# Device/OS name reported to WhatsApp
# Default: Ubuntu
WAHA_CLIENT_DEVICE_NAME=Ubuntu
```

## Keepalive

whatsmeow pings the websocket on a random interval in `[min, max)` (default 20-30s)
to keep the connection alive. Some proxies (residential/rotating) reap an idle
tunnel sooner than that, so the ping lands on an already-dead socket every time,
producing constant `Keepalive timed out` → reconnect loops (session flapping).

Lower the interval below the proxy's idle-reap window to keep the tunnel warm.
Both values are in **seconds**; `0` or unset leaves the whatsmeow default.
`max` must be strictly greater than `min` — if it isn't, `max` is adjusted to
`min + 10s` and a warning is logged.

```bash
# Ping every 8-12s instead of the default 20-30s
WAHA_GOWS_KEEPALIVE_INTERVAL_MIN_SEC=8
WAHA_GOWS_KEEPALIVE_INTERVAL_MAX_SEC=12
```

## GOWS Device Props

Low-level device capability flags sent to WhatsApp during session linking.
All variables below are **unset by default** — WhatsApp's own defaults apply unless you override them.

To unset a previously configured value explicitly, use `null`:
```bash
WAHA_GOWS_DEVICE_HISTORY_SYNC_FULL_SYNC_DAYS_LIMIT=null
```

### History Sync — Limiting sync depth

> ⚠️ These are low-level device capability flags sent to WhatsApp servers during device registration.
> Add those to env variables, restart the **container** and **repair** (logout/scan QR) a session.

By default GOWS syncs the full available WhatsApp history, which can go back many years and cause
excessive proxy bandwidth consumption and database bloat.
The following variables can be used to limit how far back historical messages are synced.

**Worth trying** — experimental env variables to limit history sync depth:

```bash
WHATSAPP_DEFAULT_ENGINE=GOWS

# Tell WhatsApp not to require a full sync
WAHA_GOWS_DEVICE_REQUIRE_FULL_SYNC=false

# Limit full sync to last N days (e.g. 365 = ~1 year)
WAHA_GOWS_DEVICE_HISTORY_SYNC_FULL_SYNC_DAYS_LIMIT=365

# Limit recent sync window
WAHA_GOWS_DEVICE_HISTORY_SYNC_RECENT_SYNC_DAYS_LIMIT=30

# Cap messages fetched per chat on initial sync
WAHA_GOWS_DEVICE_HISTORY_SYNC_INITIAL_SYNC_MAX_MESSAGES_PER_CHAT=100
```

**Secondary** — may help cap bandwidth/storage:

```bash
WHATSAPP_DEFAULT_ENGINE=GOWS

# Max MB allowed for a full sync payload
WAHA_GOWS_DEVICE_HISTORY_SYNC_FULL_SYNC_SIZE_MB_LIMIT=512

# Storage quota hint sent to WhatsApp servers
WAHA_GOWS_DEVICE_HISTORY_SYNC_STORAGE_QUOTA_MB=1024
```

### History Sync — All variables

```bash
# Require full sync on device link
# Default: unset (WhatsApp decides)
WAHA_GOWS_DEVICE_REQUIRE_FULL_SYNC=true

# Limit full sync to last N days
# Default: unset
WAHA_GOWS_DEVICE_HISTORY_SYNC_FULL_SYNC_DAYS_LIMIT=365

# Max MB for a full sync payload
# Default: unset
WAHA_GOWS_DEVICE_HISTORY_SYNC_FULL_SYNC_SIZE_MB_LIMIT=512

# Storage quota hint (MB) sent to WhatsApp servers
# Default: unset
WAHA_GOWS_DEVICE_HISTORY_SYNC_STORAGE_QUOTA_MB=1024

# Deliver initial sync payload inline in E2EE message
# Default: unset
WAHA_GOWS_DEVICE_HISTORY_SYNC_INLINE_INITIAL_PAYLOAD_IN_E2EE_MSG=true

# Limit recent sync window to last N days
# Default: unset
WAHA_GOWS_DEVICE_HISTORY_SYNC_RECENT_SYNC_DAYS_LIMIT=30

# Cap messages fetched per chat on initial sync
# Default: unset
WAHA_GOWS_DEVICE_HISTORY_SYNC_INITIAL_SYNC_MAX_MESSAGES_PER_CHAT=100

# Limit thumbnail sync to last N days
# Default: unset
WAHA_GOWS_DEVICE_HISTORY_SYNC_THUMBNAIL_SYNC_DAYS_LIMIT=90

# Feature flags (default: unset)
WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_CALL_LOG_HISTORY=true
WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_BOT_USER_AGENT_CHAT_HISTORY=true
WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_CAG_REACTIONS_AND_POLLS=true
WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_BIZ_HOSTED_MSG=true
WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_RECENT_SYNC_CHUNK_MESSAGE_COUNT_TUNING=true
WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_HOSTED_GROUP_MSG=true
WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_FBID_BOT_CHAT_HISTORY=true
WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_ADD_ON_HISTORY_SYNC_MIGRATION=true
WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_MESSAGE_ASSOCIATION=true
WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_GROUP_HISTORY=true
WAHA_GOWS_DEVICE_HISTORY_SYNC_ON_DEMAND_READY=true
WAHA_GOWS_DEVICE_HISTORY_SYNC_COMPLETE_ON_DEMAND_READY=true
WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_GUEST_CHAT=true
WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_MANUS_HISTORY=true
WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_HATCH_HISTORY=true
```
