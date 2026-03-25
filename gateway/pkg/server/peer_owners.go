package server

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/sharedredis"
)

type peerOwnershipStore interface {
	Reserve(pubKey string, ownerID string, gatewayInstanceID string, expiresAt time.Time) (bool, error)
	OwnedBy(pubKey string, ownerID string) (bool, error)
	Release(pubKey string) error
	ReleaseByOwner(ownerID string, gatewayInstanceID string) error
}

type peerReservation struct {
	ownerID           string
	gatewayInstanceID string
	expiresAt         time.Time
}

type localPeerOwnershipStore struct {
	mu         sync.RWMutex
	owners     map[string]peerReservation
	ownerIndex map[string]map[string]struct{}
}

func newLocalPeerOwnershipStore() peerOwnershipStore {
	store := &localPeerOwnershipStore{
		owners:     make(map[string]peerReservation),
		ownerIndex: make(map[string]map[string]struct{}),
	}
	go store.cleanup()
	return store
}

func (s *localPeerOwnershipStore) Reserve(
	pubKey string,
	ownerID string,
	gatewayInstanceID string,
	expiresAt time.Time,
) (bool, error) {
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.owners[pubKey]; ok {
		if now.Before(existing.expiresAt) && existing.ownerID != ownerID {
			return false, nil
		}
		s.removeIndexLocked(pubKey, existing.ownerID, existing.gatewayInstanceID)
	}

	s.owners[pubKey] = peerReservation{
		ownerID:           ownerID,
		gatewayInstanceID: gatewayInstanceID,
		expiresAt:         expiresAt,
	}
	s.addIndexLocked(pubKey, ownerID, gatewayInstanceID)
	return true, nil
}

func (s *localPeerOwnershipStore) OwnedBy(pubKey string, ownerID string) (bool, error) {
	now := time.Now()

	s.mu.RLock()
	reservation, ok := s.owners[pubKey]
	s.mu.RUnlock()
	if !ok {
		return false, nil
	}
	if now.After(reservation.expiresAt) {
		_ = s.Release(pubKey)
		return false, nil
	}
	return reservation.ownerID == ownerID, nil
}

func (s *localPeerOwnershipStore) Release(pubKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.owners[pubKey]; ok {
		s.removeIndexLocked(pubKey, existing.ownerID, existing.gatewayInstanceID)
	}
	delete(s.owners, pubKey)
	return nil
}

func (s *localPeerOwnershipStore) ReleaseByOwner(ownerID string, gatewayInstanceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	indexKey := localPeerOwnerIndexKey(ownerID, gatewayInstanceID)
	pubKeys := s.ownerIndex[indexKey]
	for pubKey := range pubKeys {
		if existing, ok := s.owners[pubKey]; ok &&
			existing.ownerID == ownerID &&
			existing.gatewayInstanceID == gatewayInstanceID {
			delete(s.owners, pubKey)
		}
	}
	delete(s.ownerIndex, indexKey)
	return nil
}

func (s *localPeerOwnershipStore) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		s.mu.Lock()
		for pubKey, reservation := range s.owners {
			if now.After(reservation.expiresAt) {
				s.removeIndexLocked(pubKey, reservation.ownerID, reservation.gatewayInstanceID)
				delete(s.owners, pubKey)
			}
		}
		s.mu.Unlock()
	}
}

func (s *localPeerOwnershipStore) addIndexLocked(pubKey string, ownerID string, gatewayInstanceID string) {
	indexKey := localPeerOwnerIndexKey(ownerID, gatewayInstanceID)
	pubKeys := s.ownerIndex[indexKey]
	if pubKeys == nil {
		pubKeys = make(map[string]struct{})
		s.ownerIndex[indexKey] = pubKeys
	}
	pubKeys[pubKey] = struct{}{}
}

func (s *localPeerOwnershipStore) removeIndexLocked(pubKey string, ownerID string, gatewayInstanceID string) {
	indexKey := localPeerOwnerIndexKey(ownerID, gatewayInstanceID)
	pubKeys := s.ownerIndex[indexKey]
	if pubKeys == nil {
		return
	}
	delete(pubKeys, pubKey)
	if len(pubKeys) == 0 {
		delete(s.ownerIndex, indexKey)
	}
}

func localPeerOwnerIndexKey(ownerID string, gatewayInstanceID string) string {
	return ownerID + "\x00" + gatewayInstanceID
}

type redisPeerOwnershipStore struct {
	store *sharedredis.Store
}

func newRedisPeerOwnershipStore(store *sharedredis.Store) peerOwnershipStore {
	return &redisPeerOwnershipStore{store: store}
}

const reservePeerScript = `
local current = redis.call("GET", KEYS[1])
local function parse_owner(value)
  local delim = string.find(value, "\n", 1, true)
  if not delim then
    return value, ""
  end
  return string.sub(value, 1, delim - 1), string.sub(value, delim + 1)
end
if current then
  local currentOwner, currentGateway = parse_owner(current)
  if currentOwner ~= ARGV[1] then
    return 0
  end
  if currentGateway ~= "" then
    redis.call("DEL", ARGV[5] .. ":" .. currentGateway .. ":" .. ARGV[6])
  end
end
redis.call("PSETEX", KEYS[1], ARGV[4], ARGV[3])
redis.call("PSETEX", KEYS[2], ARGV[4], "1")
return 1
`

func (s *redisPeerOwnershipStore) Reserve(
	pubKey string,
	ownerID string,
	gatewayInstanceID string,
	expiresAt time.Time,
) (bool, error) {
	ttlMillis := max(sharedredis.TTLUntil(expiresAt).Milliseconds(), int64(1000))
	result, err := s.store.Client().Eval(
		context.Background(),
		reservePeerScript,
		[]string{
			s.store.Key("peer-owner", pubKey),
			s.store.Key("peer-owner-session", ownerID, gatewayInstanceID, pubKey),
		},
		ownerID,
		gatewayInstanceID,
		encodePeerReservationValue(ownerID, gatewayInstanceID),
		ttlMillis,
		s.store.Key("peer-owner-session", ownerID),
		pubKey,
	).Int64()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func (s *redisPeerOwnershipStore) OwnedBy(pubKey string, ownerID string) (bool, error) {
	value, err := s.store.Client().Get(
		context.Background(),
		s.store.Key("peer-owner", pubKey),
	).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	storedOwnerID, _ := decodePeerReservationValue(value)
	return storedOwnerID == ownerID, nil
}

func (s *redisPeerOwnershipStore) Release(pubKey string) error {
	ctx := context.Background()
	value, err := s.store.Client().Get(ctx, s.store.Key("peer-owner", pubKey)).Result()
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		return err
	}

	ownerID, gatewayInstanceID := decodePeerReservationValue(value)
	keys := []string{s.store.Key("peer-owner", pubKey)}
	if ownerID != "" {
		keys = append(keys, s.store.Key("peer-owner-session", ownerID, gatewayInstanceID, pubKey))
	}
	return s.store.Client().Del(ctx, keys...).Err()
}

func (s *redisPeerOwnershipStore) ReleaseByOwner(ownerID string, gatewayInstanceID string) error {
	ctx := context.Background()
	pattern := s.store.Key("peer-owner-session", ownerID, gatewayInstanceID, "*")
	prefix := s.store.Key("peer-owner-session", ownerID, gatewayInstanceID) + ":"
	var cursor uint64

	for {
		keys, nextCursor, err := s.store.Client().Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}
		for _, key := range keys {
			pubKey := strings.TrimPrefix(key, prefix)
			if pubKey == "" {
				if err := s.store.Client().Del(ctx, key).Err(); err != nil {
					return err
				}
				continue
			}

			primaryKey := s.store.Key("peer-owner", pubKey)
			value, err := s.store.Client().Get(ctx, primaryKey).Result()
			if err == redis.Nil {
				if err := s.store.Client().Del(ctx, key).Err(); err != nil {
					return err
				}
				continue
			}
			if err != nil {
				return err
			}

			storedOwnerID, storedGatewayInstanceID := decodePeerReservationValue(value)
			delKeys := []string{key}
			if storedOwnerID == ownerID && storedGatewayInstanceID == gatewayInstanceID {
				delKeys = append(delKeys, primaryKey)
			}
			if err := s.store.Client().Del(ctx, delKeys...).Err(); err != nil {
				return err
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			return nil
		}
	}
}

func encodePeerReservationValue(ownerID string, gatewayInstanceID string) string {
	return ownerID + "\n" + gatewayInstanceID
}

func decodePeerReservationValue(value string) (string, string) {
	ownerID, gatewayInstanceID, found := strings.Cut(value, "\n")
	if !found {
		return value, ""
	}
	return ownerID, gatewayInstanceID
}
