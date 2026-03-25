package nftgate

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"

	"github.com/ethereum/go-ethereum/common"
	"github.com/redis/go-redis/v9"

	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/sharedredis"
)

type sessionStoreBackend interface {
	Set(session *Session) error
	GetByID(id string) (*Session, error)
	GetByAddress(addr common.Address) (*Session, error)
	DeleteByID(id string) error
	DeleteByAddress(addr common.Address) error
	Len() (int, error)
	BindGateway(id string, gateway GatewayIdentity) (*Session, bool, error)
	ReleaseGateway(id string, gatewayInstanceID string) error
}

type inMemorySessionBackend struct {
	store *SessionStore
}

func newInMemorySessionBackend() sessionStoreBackend {
	return &inMemorySessionBackend{store: NewSessionStore()}
}

func (b *inMemorySessionBackend) Set(session *Session) error {
	b.store.Set(session)
	return nil
}

func (b *inMemorySessionBackend) GetByID(id string) (*Session, error) {
	return b.store.GetByID(id), nil
}

func (b *inMemorySessionBackend) GetByAddress(addr common.Address) (*Session, error) {
	return b.store.GetByAddress(addr), nil
}

func (b *inMemorySessionBackend) DeleteByID(id string) error {
	b.store.DeleteByID(id)
	return nil
}

func (b *inMemorySessionBackend) DeleteByAddress(addr common.Address) error {
	b.store.DeleteByAddress(addr)
	return nil
}

func (b *inMemorySessionBackend) Len() (int, error) {
	return b.store.Len(), nil
}

func (b *inMemorySessionBackend) BindGateway(id string, gateway GatewayIdentity) (*Session, bool, error) {
	b.store.mu.Lock()
	defer b.store.mu.Unlock()

	session := b.store.sessions[id]
	if session == nil {
		return nil, false, nil
	}
	if session.GatewayInstanceID != "" && session.GatewayInstanceID != gateway.InstanceID {
		return session, false, nil
	}

	newlyBound := session.GatewayInstanceID == ""
	session.GatewayInstanceID = gateway.InstanceID
	session.GatewayPublicURL = gateway.PublicURL
	session.GatewayForwardURL = gateway.ForwardURL
	return session, newlyBound, nil
}

func (b *inMemorySessionBackend) ReleaseGateway(id string, gatewayInstanceID string) error {
	b.store.mu.Lock()
	defer b.store.mu.Unlock()

	session := b.store.sessions[id]
	if session == nil {
		return nil
	}
	if session.GatewayInstanceID != gatewayInstanceID {
		return nil
	}

	session.GatewayInstanceID = ""
	session.GatewayPublicURL = ""
	session.GatewayForwardURL = ""
	return nil
}

type redisSessionBackend struct {
	store *sharedredis.Store
}

// NewRedisSessionStore creates a Redis-backed session store for multi-node gateways.
func NewRedisSessionStore(store *sharedredis.Store) sessionStoreBackend {
	return &redisSessionBackend{store: store}
}

type GateOptions struct {
	SessionStore         sessionStoreBackend
	SessionSigningSecret string
}

func deriveSigningKey(secret string) ([32]byte, error) {
	var key [32]byte
	if secret == "" {
		if _, err := rand.Read(key[:]); err != nil {
			return key, err
		}
		return key, nil
	}

	sum := sha256.Sum256([]byte(secret))
	copy(key[:], sum[:])
	return key, nil
}

func (b *redisSessionBackend) Set(session *Session) error {
	payload, err := json.Marshal(session)
	if err != nil {
		return err
	}

	ctx := contextBackground()
	sessionKey := b.store.Key("session", "id", session.ID)
	ttl := sharedredis.TTLUntil(session.ExpiresAt)

	pipe := b.store.Client().Pipeline()
	pipe.Set(ctx, sessionKey, payload, ttl)
	if session.AddressBound {
		addressKey := b.store.Key("session", "address", session.Address.Hex())
		oldID, err := b.store.Client().Get(ctx, addressKey).Result()
		if err == nil && oldID != "" && oldID != session.ID {
			pipe.Del(ctx, b.store.Key("session", "id", oldID))
		}
		pipe.Set(ctx, addressKey, session.ID, ttl)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (b *redisSessionBackend) GetByID(id string) (*Session, error) {
	raw, err := b.store.Client().Get(contextBackground(), b.store.Key("session", "id", id)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var session Session
	if err := json.Unmarshal(raw, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func (b *redisSessionBackend) GetByAddress(addr common.Address) (*Session, error) {
	id, err := b.store.Client().Get(
		contextBackground(),
		b.store.Key("session", "address", addr.Hex()),
	).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return b.GetByID(id)
}

func (b *redisSessionBackend) DeleteByID(id string) error {
	session, err := b.GetByID(id)
	if err != nil || session == nil {
		return err
	}

	keys := []string{b.store.Key("session", "id", id)}
	if session.AddressBound {
		keys = append(keys, b.store.Key("session", "address", session.Address.Hex()))
	}
	return b.store.Client().Del(contextBackground(), keys...).Err()
}

func (b *redisSessionBackend) DeleteByAddress(addr common.Address) error {
	id, err := b.store.Client().Get(
		contextBackground(),
		b.store.Key("session", "address", addr.Hex()),
	).Result()
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		return err
	}

	return b.store.Client().Del(
		contextBackground(),
		b.store.Key("session", "address", addr.Hex()),
		b.store.Key("session", "id", id),
	).Err()
}

func (b *redisSessionBackend) Len() (int, error) {
	var (
		cursor uint64
		count  int
		ctx    = contextBackground()
	)
	pattern := b.store.Key("session", "id", "*")
	for {
		keys, nextCursor, err := b.store.Client().Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return 0, err
		}
		count += len(keys)
		cursor = nextCursor
		if cursor == 0 {
			return count, nil
		}
	}
}

func (b *redisSessionBackend) BindGateway(id string, gateway GatewayIdentity) (*Session, bool, error) {
	ctx := contextBackground()
	sessionKey := b.store.Key("session", "id", id)

	for {
		var (
			stored     *Session
			newlyBound bool
		)
		err := b.store.Client().Watch(ctx, func(tx *redis.Tx) error {
			raw, err := tx.Get(ctx, sessionKey).Bytes()
			if err == redis.Nil {
				return nil
			}
			if err != nil {
				return err
			}

			var session Session
			if err := json.Unmarshal(raw, &session); err != nil {
				return err
			}
			stored = &session
			if session.GatewayInstanceID != "" && session.GatewayInstanceID != gateway.InstanceID {
				return nil
			}

			newlyBound = session.GatewayInstanceID == ""
			if !newlyBound &&
				session.GatewayPublicURL == gateway.PublicURL &&
				session.GatewayForwardURL == gateway.ForwardURL {
				return nil
			}

			session.GatewayInstanceID = gateway.InstanceID
			session.GatewayPublicURL = gateway.PublicURL
			session.GatewayForwardURL = gateway.ForwardURL
			payload, err := json.Marshal(&session)
			if err != nil {
				return err
			}
			ttl := sharedredis.TTLUntil(session.ExpiresAt)
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, sessionKey, payload, ttl)
				if session.AddressBound {
					pipe.Set(ctx, b.store.Key("session", "address", session.Address.Hex()), session.ID, ttl)
				}
				return nil
			})
			if err != nil {
				return err
			}
			stored = &session
			return nil
		}, sessionKey)
		if err == redis.TxFailedErr {
			continue
		}
		return stored, newlyBound, err
	}
}

func (b *redisSessionBackend) ReleaseGateway(id string, gatewayInstanceID string) error {
	ctx := contextBackground()
	sessionKey := b.store.Key("session", "id", id)

	for {
		err := b.store.Client().Watch(ctx, func(tx *redis.Tx) error {
			raw, err := tx.Get(ctx, sessionKey).Bytes()
			if err == redis.Nil {
				return nil
			}
			if err != nil {
				return err
			}

			var session Session
			if err := json.Unmarshal(raw, &session); err != nil {
				return err
			}
			if session.GatewayInstanceID != gatewayInstanceID {
				return nil
			}
			if session.GatewayInstanceID == "" &&
				session.GatewayPublicURL == "" &&
				session.GatewayForwardURL == "" {
				return nil
			}

			session.GatewayInstanceID = ""
			session.GatewayPublicURL = ""
			session.GatewayForwardURL = ""
			payload, err := json.Marshal(&session)
			if err != nil {
				return err
			}
			ttl := sharedredis.TTLUntil(session.ExpiresAt)
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, sessionKey, payload, ttl)
				if session.AddressBound {
					pipe.Set(ctx, b.store.Key("session", "address", session.Address.Hex()), session.ID, ttl)
				}
				return nil
			})
			return err
		}, sessionKey)
		if err == redis.TxFailedErr {
			continue
		}
		return err
	}
}

func contextBackground() context.Context {
	return context.Background()
}
