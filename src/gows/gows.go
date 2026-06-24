package gows

import (
	"context"
	"runtime/debug"
	"sync"
	"time"

	"github.com/devlikeapro/gows/storage"
	"github.com/devlikeapro/gows/storage/sqlstorage"
	"github.com/devlikeapro/gows/voip/call"
	"github.com/jellydator/ttlcache/v3"
	_ "github.com/jackc/pgx/v5"     // Import the Postgres driver
	_ "github.com/mattn/go-sqlite3" // Import the SQLite driver
	"go.mau.fi/whatsmeow"
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

	callManager  *call.CallManager
	callBridgeMu sync.Mutex
	callBridge   callBridgeState
	callOwnersMu sync.Mutex
	callOwners   map[string]string
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

	case *CallLifecycleEvent:
		data = event

	case *events.CallOffer, *events.CallReject, *events.CallAccept,
		*events.CallTerminate, *events.CallTransport:
		// Raw whatsmeow call events are still forwarded for debugging;
		// WAHA should prefer CallLifecycleEvent payloads.
		data = event

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
	gows.routeCallEvent(event)
	go gows.reissueEvent(event)
	go gows.storageEventHandler.handleEvent(event)
}

func (gows *GoWS) Start() error {
	gows.eventHandlerID = gows.AddEventHandler(gows.handleEvent)

	// Not connected, listen for QR code events
	if gows.Store.ID == nil {
		gows.listenQRCodeEvents()
	}

	return gows.Connect()
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

	gows.cleanupCalls()

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
		nil,
		sync.Mutex{},
		callBridgeState{},
		sync.Mutex{},
		make(map[string]string),
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
