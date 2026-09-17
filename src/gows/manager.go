package gows

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	gowsLog "github.com/devlikeapro/gows/log"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/store"
	waLog "go.mau.fi/whatsmeow/util/log"
)

var ErrSessionNotFound = errors.New("session not found")

// SessionManager control sessions in thread-safe way
type SessionManager struct {
	sessions     map[string]*GoWS
	sessionsLock *sync.RWMutex
	log          waLog.Logger
}

type StoreConfig struct {
	Dialect string
	Address string
}

type StorageConfig struct {
	Messages       bool
	Groups         bool
	Chats          bool
	Labels         bool
	Contacts       bool
	MessageSecrets bool
}

func DefaultStorageConfig() StorageConfig {
	return StorageConfig{
		Messages:       true,
		Groups:         true,
		Chats:          true,
		Labels:         true,
		Contacts:       true,
		MessageSecrets: true,
	}
}

type LogConfig struct {
	Level string
}

type ProxyConfig struct {
	Url string
}

type IgnoreJidsConfig struct {
	// Status indicates whether to ignore the special status broadcast JID (status@broadcast)
	// Note: Only applies to the "status@broadcast" JID.
	Status bool
	// Groups indicate whether to ignore JIDs with server type GroupServer (g.us)
	Groups bool
	// Newsletters indicate whether to ignore JIDs with server type NewsletterServer (newsletter)
	// This corresponds to WhatsApp Channels.
	Newsletters bool
	// Broadcast indicates whether to ignore broadcast list JIDs (types.BroadcastServer),
	// excluding the special "status@broadcast" JID which is controlled by the Status flag above.
	Broadcast bool
}

// SessionConfig contains configuration for a WhatsApp session.
type SessionConfig struct {
	Store   StoreConfig
	Storage StorageConfig
	Log     LogConfig
	Proxy   ProxyConfig
	Ignore  *IgnoreJidsConfig
}

func SetDeviceAndBrowser(device string, browser string) {
	store.DeviceProps.PlatformType = browserPlatformType(browser)
	store.SetOSInfo(device, [3]uint32{22, 0, 4})
}

// statusParticipantsBatchSize is the number of contacts per batch when sending to status@broadcast.
// Set at startup via SetStatusParticipantsBatchSize; defaults to 5000.
var statusParticipantsBatchSize = 5000

func SetStatusParticipantsBatchSize(n int) {
	// lo.Chunk requires a positive size, and the batch size is now the only thing
	// deciding how a status is split, so refuse a value that would panic.
	if n <= 0 {
		return
	}
	statusParticipantsBatchSize = n
}

// statusBatchTimeout is how long to wait for the server to ack one status batch.
// Set at startup via SetStatusBatchTimeout; defaults to 180s. Zero leaves
// whatsmeow's own default (75s) in place.
var statusBatchTimeout = 180 * time.Second

func SetStatusBatchTimeout(d time.Duration) {
	statusBatchTimeout = d
}

// statusBatchDelay is the pause between status batches.
// Set at startup via SetStatusBatchDelay; defaults to 1.5s.
var statusBatchDelay = 1500 * time.Millisecond

func SetStatusBatchDelay(d time.Duration) {
	statusBatchDelay = d
}

// statusBatchMaxRetries is how many extra attempts a status batch gets after a
// transient failure. Set at startup via SetStatusBatchRetry; defaults to 2.
var statusBatchMaxRetries = 2

// statusBatchRetryBackoff is the wait before the first retry, tripling on each
// further attempt. Set at startup via SetStatusBatchRetry; defaults to 5s.
var statusBatchRetryBackoff = 5 * time.Second

func SetStatusBatchRetry(maxRetries int, backoff time.Duration) {
	if maxRetries >= 0 {
		statusBatchMaxRetries = maxRetries
	}
	if backoff > 0 {
		statusBatchRetryBackoff = backoff
	}
}

// SetKeepAliveInterval overrides whatsmeow's websocket keepalive ping interval.
// Returns the resulting min/max so the caller can log what is in effect.
func SetKeepAliveInterval(min time.Duration, max time.Duration) (time.Duration, time.Duration) {
	// Zero values leave the whatsmeow default (min 20s / max 30s) in place
	if min > 0 {
		whatsmeow.KeepAliveIntervalMin = min
	}
	if max > 0 {
		whatsmeow.KeepAliveIntervalMax = max
	}
	// whatsmeow picks a random interval in [min, max);
	// max must be strictly greater than min or rand.Int64N panics at ping time
	if whatsmeow.KeepAliveIntervalMax <= whatsmeow.KeepAliveIntervalMin {
		whatsmeow.KeepAliveIntervalMax = whatsmeow.KeepAliveIntervalMin + 10*time.Second
	}
	return whatsmeow.KeepAliveIntervalMin, whatsmeow.KeepAliveIntervalMax
}

func GetDeviceProps() *waCompanionReg.DeviceProps {
	return store.DeviceProps
}

// browserPlatformType resolves a name to a DeviceProps_PlatformType.
// Any PlatformType enum name is accepted (case-insensitive), not just browsers:
// "Desktop" renders the device name alone in WhatsApp -> Linked devices,
// while a browser name renders as "Browser (DeviceName)".
func browserPlatformType(name string) *waCompanionReg.DeviceProps_PlatformType {
	key := strings.ToUpper(strings.TrimSpace(name))
	value, ok := waCompanionReg.DeviceProps_PlatformType_value[key]
	if !ok {
		return waCompanionReg.DeviceProps_UNKNOWN.Enum()
	}
	return waCompanionReg.DeviceProps_PlatformType(value).Enum()
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions:     make(map[string]*GoWS),
		sessionsLock: &sync.RWMutex{},
		log:          gowsLog.Stdout("Manager", "DEBUG", false),
	}
}

func (sm *SessionManager) Build(name string, cfg SessionConfig) (*GoWS, error) {
	sm.sessionsLock.Lock()
	defer sm.sessionsLock.Unlock()
	gows, err := sm.unlockedBuild(name, cfg)
	if err != nil {
		sm.log.Errorf("Error building session '%s': %v", name, err)
		return nil, err
	}
	return gows, nil
}

func (sm *SessionManager) unlockedBuild(name string, cfg SessionConfig) (*GoWS, error) {
	if goWS, ok := sm.sessions[name]; ok {
		return goWS, nil
	}
	sm.log.Debugf("Building session '%s'...", name)

	ctx := context.WithValue(context.Background(), "name", name)
	log := gowsLog.Stdout("Session", cfg.Log.Level, false)

	dialect := cfg.Store.Dialect
	address := cfg.Store.Address
	gows, err := BuildSession(ctx, log.Sub(name), dialect, address, cfg.Ignore, cfg.Storage)
	if err != nil {
		return nil, err
	}
	sm.sessions[name] = gows

	err = gows.SetProxyAddress(cfg.Proxy.Url)
	if err != nil {
		delete(sm.sessions, name)
		return nil, err
	}
	sm.log.Infof("Session has been built '%s'", name)
	return gows, nil
}

func (sm *SessionManager) Start(name string) error {
	sm.log.Infof("Starting session '%s'...", name)
	sm.sessionsLock.RLock()
	goWS, ok := sm.sessions[name]
	sm.sessionsLock.RUnlock()
	if !ok {
		return ErrSessionNotFound
	}
	if err := goWS.Start(); err != nil {
		sm.log.Errorf("Error starting session '%s': %v", name, err)
		return err
	}
	sm.log.Infof("Session started '%s'", name)
	return nil
}

func (sm *SessionManager) Get(name string) (*GoWS, error) {
	sm.sessionsLock.RLock()
	defer sm.sessionsLock.RUnlock()

	if goWS, ok := sm.sessions[name]; !ok {
		return nil, ErrSessionNotFound
	} else {
		return goWS, nil
	}
}

func (sm *SessionManager) Stop(name string) {
	sm.log.Infof("Stopping session '%s'...", name)
	sm.sessionsLock.Lock()
	goWS, ok := sm.sessions[name]
	if ok {
		delete(sm.sessions, name)
	}
	sm.sessionsLock.Unlock()
	if ok {
		goWS.Stop()
	}
	sm.log.Infof("Session stopped '%s'", name)
}
