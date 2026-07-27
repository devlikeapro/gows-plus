package gows

import (
	"context"
	"errors"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/devlikeapro/gows/storage"
	"github.com/devlikeapro/gows/storage/sqlstorage"
	_ "github.com/jackc/pgx/v5" // Import the Postgres driver
	"github.com/jellydator/ttlcache/v3"
	_ "github.com/mattn/go-sqlite3" // Import the SQLite driver
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// GoWS it's Go WebSocket or WhatSapp ;)
type GoWS struct {
	*whatsmeow.Client
	int     *whatsmeow.DangerousInternalClient
	Context context.Context
	Storage *storage.Storage

	events              chan interface{}
	cancelContext       context.CancelFunc
	container           *sqlstorage.GContainer
	storageEventHandler *StorageEventHandler
	eventHandlerID      uint32
	// mediaRetryWaiters holds the active channel for an in-flight SendMediaRetryReceipt.
	mediaRetryWaiters sync.Map // types.MessageID → chan *events.MediaRetry
	// mediaRetryEvents caches every incoming *events.MediaRetry for 24 h with automatic eviction.
	// This lets subsequent download attempts reuse the fresh DirectPath without
	// sending another receipt, even if the first waiter already timed out.
	mediaRetryEvents *ttlcache.Cache[types.MessageID, *events.MediaRetry]
	// cappingFetchMu/cappingFetchLast throttle the message capping re-fetches
	// driven by sends and the periodic poller (connect/timelock/463 bypass it).
	cappingFetchMu    sync.Mutex
	cappingFetchLast  time.Time
	cappingPollerOnce sync.Once
}

func (gows *GoWS) reissueEvent(event interface{}) {
	// Handle all panic and log error + stack
	defer func() {
		if err := recover(); err != nil {
			stack := debug.Stack()
			gows.Log.Errorf("Panic happened in reissue event: %v. Stack: %s. Event: %v", err, stack, event)
		}
	}()

	var data interface{}
	switch event.(type) {
	case *events.Connected:
		// Populate the ConnectedEventData with the ID and PushName
		data = &ConnectedEventData{
			ID:       gows.Store.ID,
			LID:      &gows.Store.LID,
			PushName: gows.Store.PushName,
		}
		// Actively fetch the current reachout timelock state, so the session
		// learns it right after (re)start without waiting for a push notification.
		go gows.fetchReachoutTimelock()
		// Same for the new-chat message capping (there is no push for it).
		go gows.fetchMessageCapping()

	case *events.NotifyAccountReachoutTimelock:
		// The timelock and the new-chat capping are siblings in WhatsApp's
		// cold-outreach limiting, so refresh the capping whenever the timelock
		// changes (WhatsApp pushes no capping notification of its own).
		go gows.fetchMessageCapping()
		data = event

	case *events.Message:
		msg := event.(*events.Message)
		sem := msg.Message.GetSecretEncryptedMessage()
		if sem != nil && sem.GetSecretEncType() == waE2E.SecretEncryptedMessage_MESSAGE_EDIT {
			go gows.handleSecretMessageEdit(gows.Context, msg)
			return
		} else if msg.Message.GetEncEventResponseMessage() != nil {
			data = event
			go gows.handleEncEventResponse(gows.Context, msg)
		} else if msg.Message.GetPollUpdateMessage() != nil {
			data = event
			go gows.handleEncPollVote(gows.Context, msg)
		} else {
			data = event
		}

	case *events.MediaRetry:
		evt := event.(*events.MediaRetry)
		// Always cache so that callers whose 60 s wait already expired can still
		// pick up the result on their next NestJS-level retry.
		gows.mediaRetryEvents.Set(evt.MessageID, evt, ttlcache.DefaultTTL)
		// Notify any goroutine that is still actively waiting.
		if ch, loaded := gows.mediaRetryWaiters.Load(evt.MessageID); loaded {
			select {
			case ch.(chan *events.MediaRetry) <- evt:
			default:
			}
		}
		data = event

	default:
		data = event
	}

	gows.emitEvent(data)
}

func (gows *GoWS) handleEvent(event interface{}) {
	go gows.reissueEvent(event)
	go gows.storageEventHandler.handleEvent(event)
}

func (gows *GoWS) Start() error {
	// Guard against double-registration if Start is called more than once without Stop.
	// AddEventHandler appends without checking for existing handlers, so a stale
	// handler would leak and cause every event to be emitted twice into gows.events.
	if gows.eventHandlerID != 0 {
		gows.RemoveEventHandler(gows.eventHandlerID)
	}
	gows.eventHandlerID = gows.AddEventHandler(gows.handleEvent)

	// Start the periodic message capping poller once for the session lifetime.
	gows.cappingPollerOnce.Do(func() {
		go gows.pollMessageCapping()
	})

	// Not connected, listen for QR code events
	if gows.Store.ID == nil {
		gows.listenQRCodeEvents()
	}

	if err := gows.Connect(); err != nil {
		return err
	}

	// Ensure the NCT salt is populated for already-registered sessions.
	// Sessions upgraded to DB schema v14 have an empty whatsmeow_nct_salt table.
	// The regular on-connect FetchAppState uses onlyIfNotSynced=true and skips
	// regular_high when its version is already > 0, so the salt is never written.
	// Without the salt, generateCsToken returns nil; if tctoken is also absent,
	// WhatsApp rejects every outbound DM with error 400 until the session is
	// restarted (which triggers handleAppStateNotification with onlyIfNotSynced=false).
	// We reproduce that forced re-sync here so the session heals on its own.
	if gows.Store.ID != nil {
		go gows.ensureNCTSalt()
	}

	return nil
}

// ensureNCTSalt forces a regular_high app-state sync when the NCT salt table
// is empty. It waits a short time after Connect() to let whatsmeow complete its
// own post-connect sync before checking.
func (gows *GoWS) ensureNCTSalt() {
	select {
	case <-gows.Context.Done():
		return
	case <-time.After(5 * time.Second):
	}

	salt, err := gows.Store.NCTSalt.GetNCTSalt(gows.Context)
	if err != nil {
		gows.Log.Errorf("Failed to read NCT salt: %v", err)
		return
	}
	if len(salt) > 0 {
		return
	}

	gows.Log.Infof("NCT salt is empty — forcing regular_high app-state sync")
	if err := gows.FetchAppState(gows.Context, appstate.WAPatchRegularHigh, false, false); err != nil {
		gows.Log.Errorf("Failed to force regular_high app-state sync: %v", err)
	}
}

// fetchReachoutTimelock fetches the current account reachout timelock state and
// re-emits it as *events.NotifyAccountReachoutTimelock, so the API side handles
// the fetched state exactly like the push notification.
func (gows *GoWS) fetchReachoutTimelock() {
	ctx, cancel := context.WithTimeout(gows.Context, 30*time.Second)
	defer cancel()

	result, err := gows.FetchAccountReachoutTimelock(ctx)
	if err != nil {
		gows.Log.Errorf("Failed to fetch account reachout timelock: %v", err)
		return
	}
	gows.emitEvent(result)
}

// Message capping re-fetch cadence.
const (
	// cappingMinFetchInterval throttles the send-driven and periodic fetches so
	// a burst of sends does not query the server on every message.
	cappingMinFetchInterval = 60 * time.Second
	// cappingPollInterval is the periodic safety-net re-fetch while connected.
	cappingPollInterval = 10 * time.Minute
)

// fetchMessageCapping fetches the current new-chat message capping state and
// re-emits it as *MessageCapping, so the API side can track the account's
// per-cycle quota. Called (bypassing the throttle) on connect, whenever the
// reachout timelock changes, and when a send is rejected with error 463.
func (gows *GoWS) fetchMessageCapping() {
	gows.cappingFetchMu.Lock()
	gows.cappingFetchLast = time.Now()
	gows.cappingFetchMu.Unlock()

	ctx, cancel := context.WithTimeout(gows.Context, 30*time.Second)
	defer cancel()

	result, err := gows.FetchMessageCapping(ctx, MessageCappingTypeNewChatThread)
	if err != nil {
		gows.Log.Errorf("Failed to fetch message capping: %v", err)
		return
	}
	gows.emitEvent(result)
}

// fetchMessageCappingThrottled fetches the capping only if the last fetch is
// older than cappingMinFetchInterval. Used by the send and periodic triggers,
// which fire often and only need an approximately fresh value.
func (gows *GoWS) fetchMessageCappingThrottled() {
	gows.cappingFetchMu.Lock()
	recent := time.Since(gows.cappingFetchLast) < cappingMinFetchInterval
	gows.cappingFetchMu.Unlock()
	if recent {
		return
	}
	gows.fetchMessageCapping()
}

// pollMessageCapping re-fetches the capping periodically while connected, as a
// safety net for state that changes without a send (e.g. the cycle resetting).
func (gows *GoWS) pollMessageCapping() {
	ticker := time.NewTicker(cappingPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-gows.Context.Done():
			return
		case <-ticker.C:
			gows.fetchMessageCappingThrottled()
		}
	}
}

func (gows *GoWS) listenQRCodeEvents() {
	// No ID stored, new login
	qrChan, _ := gows.GetQRChannel(gows.Context)

	// reissue from QrChan to events
	go func() {
		for {
			select {
			case <-gows.Context.Done():
				return
			case qr := <-qrChan:
				// If the event is empty, we should stop the goroutine
				if qr.Event == "" {
					return
				}
				gows.emitEvent(qr)
			}
		}
	}()
}

func (gows *GoWS) Stop() {
	if gows == nil {
		return
	}

	// Prevent auto-reconnect and stop event emission before tearing down storage.
	gows.EnableAutoReconnect = false
	gows.InitialAutoReconnect = false
	if gows.eventHandlerID != 0 {
		gows.RemoveEventHandler(gows.eventHandlerID)
	}

	gows.Disconnect()
	if gows.mediaRetryEvents != nil {
		gows.mediaRetryEvents.Stop()
	}
	if gows.container != nil {
		err := gows.container.Close()
		if err != nil {
			gows.Log.Errorf("Error closing container: %v", err)
		}
	}
	if gows.events != nil {
		close(gows.events)
		gows.events = nil
	}
	gows.cancelContext()
}

func (gows *GoWS) GetOwnId() types.JID {
	if gows == nil {
		return types.EmptyJID
	}
	id := gows.Store.ID
	if id == nil {
		return types.EmptyJID
	}
	return *id
}

func BuildSession(
	ctx context.Context,
	log waLog.Logger,
	dialect string,
	address string,
	ignoreJids *IgnoreJidsConfig,
	storageCfg StorageConfig,
) (*GoWS, error) {
	// Prepare the database
	container, err := sqlstorage.New(dialect, address, log.Sub("Database"))
	if err != nil {
		return nil, err
	}
	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		_ = container.Close()
		return nil, err
	}

	// Configure the client
	client := whatsmeow.NewClient(deviceStore, log.Sub("Client"))
	client.AutomaticMessageRerequestFromPhone = true
	client.EmitAppStateEventsOnFullSync = true
	client.InitialAutoReconnect = true

	retryEventsCache := ttlcache.New[types.MessageID, *events.MediaRetry](
		ttlcache.WithTTL[types.MessageID, *events.MediaRetry](24 * time.Hour),
	)
	go retryEventsCache.Start()

	ctx, cancel := context.WithCancel(ctx)
	gows := &GoWS{
		client,
		client.DangerousInternals(),
		ctx,
		nil,
		make(chan interface{}, 10),
		cancel,
		container,
		nil,
		0,
		sync.Map{},
		retryEventsCache,
		sync.Mutex{},
		time.Time{},
		sync.Once{},
	}
	if storageCfg == (StorageConfig{}) {
		storageCfg = DefaultStorageConfig()
	}
	gows.Storage = BuildStorage(container, gows, storageCfg)
	gows.storageEventHandler = &StorageEventHandler{
		gows:       gows,
		log:        gows.Log.Sub("Storage"),
		storage:    gows.Storage,
		ignoreJids: ignoreJids,
	}
	gows.GetMessageForRetry = gows.storageEventHandler.GetMessageForRetry
	gows.BackgroundEventCtx = gows.Context
	return gows, nil
}

func (gows *GoWS) GetEventChannel() <-chan interface{} {
	return gows.events
}

func (gows *GoWS) emitEvent(data interface{}) {
	// Handle all panic and log error + stack
	defer func() {
		if err := recover(); err != nil {
			stack := debug.Stack()
			gows.Log.Errorf("Panic happened in emit event: %v. Stack: %s. Data: %v", err, stack, data)
		}
	}()

	select {
	case <-gows.Context.Done():
		return
	case gows.events <- data:
	}
}

func (gows *GoWS) SendMessage(ctx context.Context, to types.JID, msg *waE2E.Message, extra whatsmeow.SendRequestExtra) (message *events.Message, err error) {
	var resp whatsmeow.SendResponse

	if to.User == "status" && to.Server == types.BroadcastServer {
		// Broadcast messages (Status)
		result, err := gows.SendStatusMessage(ctx, to, msg, extra)
		if err != nil {
			return nil, err
		}
		resp = *result
	} else {
		resp, err = gows.Client.SendMessage(ctx, to, msg, extra)
		if err != nil {
			// Error 463 is NackCallerReachoutTimelocked: the account just hit
			// its cold-outreach limit, so refresh the capping to reflect it.
			if errors.Is(err, whatsmeow.ErrServerReturnedError) &&
				strings.HasSuffix(err.Error(), " 463") {
				go gows.fetchMessageCapping()
			}
			return nil, err
		}
	}

	info := &types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:     to,
			Sender:   gows.GetOwnId(),
			IsFromMe: true,
			IsGroup:  to.Server == types.GroupServer,
		},
		ID:        resp.ID,
		Timestamp: resp.Timestamp,
		ServerID:  resp.ServerID,
	}
	evt := &events.Message{Info: *info, Message: msg, RawMessage: msg}
	go gows.handleEvent(evt)
	return evt, nil
}

// MarkRead marks messages as read and emits a receipt event
func (gows *GoWS) MarkRead(ids []types.MessageID, chat types.JID, sender types.JID, receiptType types.ReceiptType) error {
	timestamp := time.Now()
	err := gows.Client.MarkRead(gows.Context, ids, timestamp, chat, sender, receiptType)
	if err != nil {
		return err
	}

	receipt := &events.Receipt{
		MessageSource: types.MessageSource{
			Chat:     chat,
			Sender:   sender,
			IsFromMe: true,
		},
		MessageIDs: ids,
		Type:       receiptType,
		Timestamp:  timestamp,
	}
	go gows.handleEvent(receipt)
	return nil
}
