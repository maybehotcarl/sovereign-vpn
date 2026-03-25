package server

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/sharedredis"
)

const gatewayPresenceTTL = 15 * time.Second

type gatewayPresence struct {
	InstanceID string    `json:"instance_id"`
	PublicURL  string    `json:"public_url,omitempty"`
	UpdatedAt  time.Time `json:"updated_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type gatewayPresenceStore interface {
	Put(presence *gatewayPresence) error
	Get(instanceID string) (*gatewayPresence, error)
	Delete(instanceID string) error
}

type localGatewayPresenceStore struct {
	mu        sync.RWMutex
	presences map[string]*gatewayPresence
}

func newLocalGatewayPresenceStore() gatewayPresenceStore {
	store := &localGatewayPresenceStore{
		presences: make(map[string]*gatewayPresence),
	}
	go store.cleanup()
	return store
}

func (s *localGatewayPresenceStore) Put(presence *gatewayPresence) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copyPresence := *presence
	s.presences[presence.InstanceID] = &copyPresence
	return nil
}

func (s *localGatewayPresenceStore) Get(instanceID string) (*gatewayPresence, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	presence := s.presences[instanceID]
	if presence == nil {
		return nil, nil
	}
	copyPresence := *presence
	return &copyPresence, nil
}

func (s *localGatewayPresenceStore) Delete(instanceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.presences, instanceID)
	return nil
}

func (s *localGatewayPresenceStore) cleanup() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now().UTC()
		s.mu.Lock()
		for instanceID, presence := range s.presences {
			if presence == nil || !presence.ExpiresAt.After(now) {
				delete(s.presences, instanceID)
			}
		}
		s.mu.Unlock()
	}
}

type redisGatewayPresenceStore struct {
	store *sharedredis.Store
}

func newRedisGatewayPresenceStore(store *sharedredis.Store) gatewayPresenceStore {
	return &redisGatewayPresenceStore{store: store}
}

func (s *redisGatewayPresenceStore) Put(presence *gatewayPresence) error {
	payload, err := json.Marshal(presence)
	if err != nil {
		return err
	}
	return s.store.Client().Set(
		context.Background(),
		s.store.Key("gateway-presence", presence.InstanceID),
		payload,
		sharedredis.TTLUntil(presence.ExpiresAt),
	).Err()
}

func (s *redisGatewayPresenceStore) Get(instanceID string) (*gatewayPresence, error) {
	raw, err := s.store.Client().Get(
		context.Background(),
		s.store.Key("gateway-presence", instanceID),
	).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var presence gatewayPresence
	if err := json.Unmarshal(raw, &presence); err != nil {
		return nil, err
	}
	return &presence, nil
}

func (s *redisGatewayPresenceStore) Delete(instanceID string) error {
	return s.store.Client().Del(
		context.Background(),
		s.store.Key("gateway-presence", instanceID),
	).Err()
}

func (s *Server) refreshGatewayPresence() error {
	if s == nil || s.gatewayPresence == nil {
		return nil
	}

	now := time.Now().UTC()
	return s.gatewayPresence.Put(&gatewayPresence{
		InstanceID: s.currentGatewayInstanceID(),
		PublicURL:  s.currentGatewayPublicURL(),
		UpdatedAt:  now,
		ExpiresAt:  now.Add(gatewayPresenceTTL),
	})
}

func (s *Server) startGatewayPresenceHeartbeat() error {
	if s == nil || s.gatewayPresence == nil {
		return nil
	}
	if err := s.refreshGatewayPresence(); err != nil {
		return err
	}
	if s.gatewayPresenceStopCh != nil {
		return nil
	}

	s.gatewayPresenceStopCh = make(chan struct{})
	interval := gatewayPresenceTTL / 3
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = s.refreshGatewayPresence()
			case <-s.gatewayPresenceStopCh:
				return
			}
		}
	}()
	return nil
}

func (s *Server) stopGatewayPresenceHeartbeat() {
	if s == nil || s.gatewayPresence == nil {
		return
	}
	if s.gatewayPresenceStopCh != nil {
		close(s.gatewayPresenceStopCh)
		s.gatewayPresenceStopCh = nil
	}
	_ = s.gatewayPresence.Delete(s.currentGatewayInstanceID())
}
