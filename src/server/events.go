package server

import (
	"encoding/json"

	"github.com/devlikeapro/gows/gows"
	"github.com/devlikeapro/gows/proto"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"reflect"
	"strings"
)

func (s *Server) safeMarshal(v interface{}) (result string) {
	defer func() {
		if err := recover(); err != nil {
			// Print log error and ignore
			s.log.Errorf("Panic happened when marshaling data: %v", err)
			result = ""
		}
	}()
	data, err := json.Marshal(v)
	if err != nil {
		s.log.Errorf("Error when marshaling data: %v", err)
		return ""
	}
	result = string(data)
	return result
}

func (s *Server) StreamEvents(req *__.StreamEventsRequest, stream grpc.ServerStreamingServer[__.EventJson]) error {
	sessionName := req.GetSession().GetId()
	streamId := uuid.New()
	listener := s.addListener(sessionName, streamId)
	defer s.removeListener(sessionName, streamId)
	exclude := make(map[string]struct{}, len(req.GetExclude()))
	for _, eventType := range req.GetExclude() {
		exclude[eventType] = struct{}{}
	}
	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case event, ok := <-listener:
			if !ok {
				return nil
			}
			if event == nil {
				continue
			}
			eventType, jsonString := s.formatStreamEvent(event)
			if jsonString == "" {
				continue
			}
			if _, ok := exclude[eventType]; ok {
				continue
			}

			data := __.EventJson{
				Session: sessionName,
				Event:   eventType,
				Data:    jsonString,
			}
			err := stream.Send(&data)
			if err != nil {
				return err
			}
		}
	}
}

func (s *Server) SendEventToAllListeners(session string, event interface{}) {
	s.listenersLock.RLock()
	defer s.listenersLock.RUnlock()

	sessionListeners := s.listeners[session]
	for id, listener := range sessionListeners {
		// Drop the event if the listener buffer is full to avoid blocking and goroutine leaks.
		select {
		case listener <- event:
		default:
			s.log.Warnf("Dropping event for slow listener %s, session %s", id.String(), session)
		}
	}
}

func (s *Server) addListener(session string, id uuid.UUID) chan interface{} {
	s.listenersLock.Lock()
	defer s.listenersLock.Unlock()

	listener := make(chan interface{}, 1000)
	sessionListeners, ok := s.listeners[session]
	if !ok {
		sessionListeners = map[uuid.UUID]chan interface{}{}
		s.listeners[session] = sessionListeners
	}
	sessionListeners[id] = listener
	return listener
}

func (s *Server) removeListener(session string, id uuid.UUID) {
	s.listenersLock.Lock()
	defer s.listenersLock.Unlock()
	listener, ok := s.listeners[session][id]
	if !ok {
		return
	}
	delete(s.listeners[session], id)
	// if it's the last listener, remove the session
	if len(s.listeners[session]) == 0 {
		delete(s.listeners, session)
	}
	close(listener)
}

func (s *Server) formatStreamEvent(event interface{}) (eventType string, jsonString string) {
	if ce, ok := event.(*gows.CallLifecycleEvent); ok {
		return ce.Event, s.safeMarshal(ce)
	}
	eventType = reflect.TypeOf(event).String()
	eventType = strings.TrimPrefix(eventType, "*")
	return eventType, s.safeMarshal(event)
}
