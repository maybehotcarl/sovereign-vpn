package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/sharedredis"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/wireguard"
)

type peerState struct {
	PublicKey         string    `json:"public_key"`
	SessionID         string    `json:"session_id"`
	GatewayInstanceID string    `json:"gateway_instance_id"`
	GatewayURL        string    `json:"gateway_url,omitempty"`
	ClientIP          string    `json:"client_ip"`
	AssignedAt        time.Time `json:"assigned_at"`
	ExpiresAt         time.Time `json:"expires_at"`
}

type peerStateStore interface {
	Put(state *peerState) error
	Get(publicKey string) (*peerState, error)
	Delete(publicKey string) error
	ListByGateway(gatewayInstanceID string) ([]*peerState, error)
	ListBySession(sessionID string) ([]*peerState, error)
}

type localPeerStateStore struct {
	mu    sync.RWMutex
	peers map[string]*peerState
}

func newLocalPeerStateStore() peerStateStore {
	store := &localPeerStateStore{
		peers: make(map[string]*peerState),
	}
	go store.cleanup()
	return store
}

func (s *localPeerStateStore) Put(state *peerState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copyState := *state
	s.peers[state.PublicKey] = &copyState
	return nil
}

func (s *localPeerStateStore) Get(publicKey string) (*peerState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state := s.peers[publicKey]
	if state == nil {
		return nil, nil
	}
	copyState := *state
	return &copyState, nil
}

func (s *localPeerStateStore) Delete(publicKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.peers, publicKey)
	return nil
}

func (s *localPeerStateStore) ListByGateway(gatewayInstanceID string) ([]*peerState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	states := make([]*peerState, 0, len(s.peers))
	for _, state := range s.peers {
		if state == nil || state.GatewayInstanceID != gatewayInstanceID {
			continue
		}
		copyState := *state
		states = append(states, &copyState)
	}
	return states, nil
}

func (s *localPeerStateStore) ListBySession(sessionID string) ([]*peerState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	states := make([]*peerState, 0, len(s.peers))
	for _, state := range s.peers {
		if state == nil || state.SessionID != sessionID {
			continue
		}
		copyState := *state
		states = append(states, &copyState)
	}
	return states, nil
}

func (s *localPeerStateStore) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		s.mu.Lock()
		for publicKey, state := range s.peers {
			if state == nil || now.After(state.ExpiresAt) {
				delete(s.peers, publicKey)
			}
		}
		s.mu.Unlock()
	}
}

type redisPeerStateStore struct {
	store *sharedredis.Store
}

func newRedisPeerStateStore(store *sharedredis.Store) peerStateStore {
	return &redisPeerStateStore{store: store}
}

func (s *redisPeerStateStore) Put(state *peerState) error {
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}

	ttl := sharedredis.TTLUntil(state.ExpiresAt)
	ctx := context.Background()
	metaKey := s.store.Key("peer-state", "public-key", state.PublicKey)
	indexKey := s.store.Key("peer-state", "gateway", state.GatewayInstanceID, state.PublicKey)
	sessionIndexKey := s.store.Key("peer-state", "session", state.SessionID, state.PublicKey)

	pipe := s.store.Client().Pipeline()
	pipe.Set(ctx, metaKey, payload, ttl)
	pipe.Set(ctx, indexKey, "1", ttl)
	pipe.Set(ctx, sessionIndexKey, "1", ttl)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *redisPeerStateStore) Get(publicKey string) (*peerState, error) {
	raw, err := s.store.Client().Get(
		context.Background(),
		s.store.Key("peer-state", "public-key", publicKey),
	).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var state peerState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func (s *redisPeerStateStore) Delete(publicKey string) error {
	state, err := s.Get(publicKey)
	if err != nil {
		return err
	}

	keys := []string{s.store.Key("peer-state", "public-key", publicKey)}
	if state != nil && state.GatewayInstanceID != "" {
		keys = append(keys, s.store.Key("peer-state", "gateway", state.GatewayInstanceID, publicKey))
	}
	if state != nil && state.SessionID != "" {
		keys = append(keys, s.store.Key("peer-state", "session", state.SessionID, publicKey))
	}
	return s.store.Client().Del(context.Background(), keys...).Err()
}

func (s *redisPeerStateStore) ListByGateway(gatewayInstanceID string) ([]*peerState, error) {
	ctx := context.Background()
	pattern := s.store.Key("peer-state", "gateway", gatewayInstanceID, "*")
	states := []*peerState{}
	var cursor uint64

	for {
		keys, nextCursor, err := s.store.Client().Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			publicKey := strings.TrimPrefix(key, s.store.Key("peer-state", "gateway", gatewayInstanceID)+":")
			if publicKey == "" {
				continue
			}
			state, err := s.Get(publicKey)
			if err != nil {
				return nil, err
			}
			if state == nil || state.GatewayInstanceID != gatewayInstanceID {
				continue
			}
			states = append(states, state)
		}
		cursor = nextCursor
		if cursor == 0 {
			return states, nil
		}
	}
}

func (s *redisPeerStateStore) ListBySession(sessionID string) ([]*peerState, error) {
	ctx := context.Background()
	pattern := s.store.Key("peer-state", "session", sessionID, "*")
	states := []*peerState{}
	var cursor uint64

	for {
		keys, nextCursor, err := s.store.Client().Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			publicKey := strings.TrimPrefix(key, s.store.Key("peer-state", "session", sessionID)+":")
			if publicKey == "" {
				continue
			}
			state, err := s.Get(publicKey)
			if err != nil {
				return nil, err
			}
			if state == nil || state.SessionID != sessionID {
				continue
			}
			states = append(states, state)
		}
		cursor = nextCursor
		if cursor == 0 {
			return states, nil
		}
	}
}

func (s *Server) recordPeerState(sessionID string, publicKey string, clientAddress string, expiresAt time.Time) error {
	if s == nil || s.peerStates == nil {
		return nil
	}

	clientIP, _, found := strings.Cut(strings.TrimSpace(clientAddress), "/")
	if !found || strings.TrimSpace(clientIP) == "" {
		clientIP = strings.TrimSpace(clientAddress)
	}
	if clientIP == "" {
		return fmt.Errorf("client address is required to persist peer state")
	}

	return s.peerStates.Put(&peerState{
		PublicKey:         publicKey,
		SessionID:         sessionID,
		GatewayInstanceID: s.currentGatewayInstanceID(),
		GatewayURL:        s.currentGatewayPublicURL(),
		ClientIP:          clientIP,
		AssignedAt:        time.Now().UTC(),
		ExpiresAt:         expiresAt.UTC(),
	})
}

func (s *Server) deletePeerState(publicKey string) error {
	if s == nil || s.peerStates == nil {
		return nil
	}
	return s.peerStates.Delete(publicKey)
}

func (s *Server) recoverOwnedPeers() error {
	if s == nil || s.peerStates == nil || s.wg == nil {
		return nil
	}

	states, err := s.peerStates.ListByGateway(s.currentGatewayInstanceID())
	if err != nil {
		return fmt.Errorf("listing peer recovery state: %w", err)
	}

	now := time.Now().UTC()
	for _, state := range states {
		if state == nil || state.PublicKey == "" || state.ClientIP == "" {
			continue
		}
		if !state.ExpiresAt.After(now) {
			_ = s.deletePeerState(state.PublicKey)
			continue
		}
		if err := s.wg.RecoverPeer(wireguard.Peer{
			PublicKey:  state.PublicKey,
			ClientIP:   state.ClientIP,
			AssignedAt: state.AssignedAt,
			ExpiresAt:  state.ExpiresAt,
		}); err != nil {
			return fmt.Errorf("recovering peer %s: %w", state.PublicKey, err)
		}
	}

	return nil
}

func (s *Server) recoverPeerIfNeeded(publicKey string) error {
	if s == nil || s.peerStates == nil || s.wg == nil || strings.TrimSpace(publicKey) == "" {
		return nil
	}
	if s.wg.GetPeer(publicKey) != nil {
		return nil
	}

	state, err := s.peerStates.Get(publicKey)
	if err != nil {
		return err
	}
	if state == nil {
		return nil
	}
	if state.GatewayInstanceID != s.currentGatewayInstanceID() {
		return nil
	}
	if !state.ExpiresAt.After(time.Now().UTC()) {
		_ = s.deletePeerState(publicKey)
		return nil
	}

	if err := s.wg.RecoverPeer(wireguard.Peer{
		PublicKey:  state.PublicKey,
		ClientIP:   state.ClientIP,
		AssignedAt: state.AssignedAt,
		ExpiresAt:  state.ExpiresAt,
	}); err != nil {
		return err
	}
	log.Printf("[wireguard] Peer state recovered on demand")
	return nil
}
