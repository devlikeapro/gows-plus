package server

import (
	"context"
	"errors"
	"github.com/devlikeapro/gows/gows"
	"github.com/devlikeapro/gows/proto"
	"go.mau.fi/whatsmeow"
	"net/url"
)

func addApplicationName(address string, name string) string {
	parsedURL, err := url.Parse(address)
	if err != nil {
		return address
	}
	queryParams := parsedURL.Query()
	queryParams.Set("application_name", name)
	parsedURL.RawQuery = queryParams.Encode()
	return parsedURL.String()
}

func (s *Server) StartSession(ctx context.Context, req *__.StartSessionRequest) (*__.Empty, error) {
	if req == nil || req.Config == nil || req.Config.Store == nil {
		return nil, errors.New("missing session config")
	}
	dialect := req.Config.Store.Dialect
	var address string
	switch {
	case dialect == "sqlite3" || dialect == "sqlite":
		// busy_timeout to prevent "database is locked" errors
		// DO NOT add cache=shared, it's not safe
		//
		// WAL is required here, not optional. In the default rollback-journal mode a
		// single writer takes an exclusive lock that blocks every other reader and
		// writer. During a status@broadcast the outgoing send and the flood of
		// incoming decryptions (each a session/identity/sender-key write) contend for
		// that one lock, exhaust the busy_timeout above and surface as "database is
		// locked". WAL lets one writer proceed alongside readers, and
		// _txlock=immediate takes the write lock up front so concurrent writers back
		// off cleanly instead of deadlocking mid-transaction.
		// (Mirrors the PRAGMAs already applied to the GContainer store.)
		address = req.Config.Store.Address + "?_foreign_keys=on&_busy_timeout=30000&_journal_mode=WAL&_synchronous=NORMAL&_txlock=immediate"
	case dialect == "postgres":
		address = addApplicationName(req.Config.Store.Address, "GOWS")
	default:
		return nil, errors.New("unsupported sql dialect: " + dialect)
	}

	cfg := gows.SessionConfig{
		Store: gows.StoreConfig{
			Dialect: dialect,
			Address: address,
		},
		Storage: gows.DefaultStorageConfig(),
		Log: gows.LogConfig{
			Level: req.Config.Log.Level.String(),
		},
		Proxy: gows.ProxyConfig{
			Url: req.Config.Proxy.Url,
		},
	}

	// Set ignore config if provided
	if req.Config.Ignore != nil {
		cfg.Ignore = &gows.IgnoreJidsConfig{
			Status:      req.Config.Ignore.Status,
			Groups:      req.Config.Ignore.Groups,
			Newsletters: req.Config.Ignore.Newsletters,
			Broadcast:   req.Config.Ignore.Broadcast,
		}
	}

	if req.Config.Storage != nil {
		if req.Config.Storage.Messages != nil {
			cfg.Storage.Messages = req.Config.Storage.GetMessages()
		}
		if req.Config.Storage.Groups != nil {
			cfg.Storage.Groups = req.Config.Storage.GetGroups()
		}
		if req.Config.Storage.Chats != nil {
			cfg.Storage.Chats = req.Config.Storage.GetChats()
		}
		if req.Config.Storage.Labels != nil {
			cfg.Storage.Labels = req.Config.Storage.GetLabels()
		}
		if req.Config.Storage.Contacts != nil {
			cfg.Storage.Contacts = req.Config.Storage.GetContacts()
		}
		if req.Config.Storage.MessageSecrets != nil {
			cfg.Storage.MessageSecrets = req.Config.Storage.GetMessageSecrets()
		}
	}

	session := req.GetId()
	cli, err := s.Sm.Build(session, cfg)
	if err != nil {
		return nil, err
	}

	s.eventSubsLock.Lock()
	_, subscribed := s.eventSubs[session]
	if !subscribed {
		subCtx, cancel := context.WithCancel(context.Background())
		s.eventSubs[session] = cancel
		// Subscribe to events
		go func() {
			eventCh := cli.GetEventChannel()
			for {
				select {
				case <-subCtx.Done():
					return
				case evt, ok := <-eventCh:
					if !ok {
						return
					}
					s.SendEventToAllListeners(session, evt)
				}
			}
		}()
	}
	s.eventSubsLock.Unlock()

	if startErr := s.Sm.Start(session); startErr != nil {
		s.log.Errorf("Error starting session '%s': %v", session, startErr)
		return nil, startErr
	}

	return &__.Empty{}, nil
}

func (s *Server) StopSession(ctx context.Context, req *__.Session) (*__.Empty, error) {
	session := req.GetId()
	s.eventSubsLock.Lock()
	cancel, ok := s.eventSubs[session]
	if ok {
		delete(s.eventSubs, session)
	}
	s.eventSubsLock.Unlock()
	if ok {
		cancel()
	}
	s.Sm.Stop(session)
	return &__.Empty{}, nil
}

func (s *Server) GetSessionState(ctx context.Context, req *__.Session) (*__.SessionStateResponse, error) {
	cli, err := s.Sm.Get(req.GetId())
	if errors.Is(err, gows.ErrSessionNotFound) {
		return &__.SessionStateResponse{Found: false, Connected: false}, nil
	}
	if err != nil {
		return nil, err
	}
	return &__.SessionStateResponse{Found: true, Connected: cli.IsConnected()}, nil
}

func (s *Server) RequestCode(ctx context.Context, req *__.PairCodeRequest) (*__.PairCodeResponse, error) {
	cli, err := s.Sm.Get(req.GetSession().GetId())
	if err != nil {
		return nil, err
	}
	code, err := cli.PairPhone(
		ctx,
		req.GetPhone(),
		true,
		whatsmeow.PairClientChrome,
		"Chrome (Linux)",
	)
	if err != nil {
		return nil, err
	}
	return &__.PairCodeResponse{Code: code}, nil
}

func (s *Server) Logout(ctx context.Context, req *__.Session) (*__.Empty, error) {
	cli, err := s.Sm.Get(req.GetId())
	if err != nil {
		return nil, err
	}
	err = cli.Logout(ctx)
	if err != nil {
		if errors.Is(err, whatsmeow.ErrNotLoggedIn) {
			// Ignore not logged in error
			return &__.Empty{}, nil
		}
		return nil, err
	}
	return &__.Empty{}, nil
}
