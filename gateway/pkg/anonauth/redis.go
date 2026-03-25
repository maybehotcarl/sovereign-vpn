package anonauth

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/sharedredis"
)

type redisChallengeBackend struct {
	store *sharedredis.Store
}

// NewRedisChallengeBackend creates a Redis-backed anonymous challenge store.
func NewRedisChallengeBackend(store *sharedredis.Store) challengeStoreBackend {
	return &redisChallengeBackend{store: store}
}

func (b *redisChallengeBackend) Set(challenge *Challenge) error {
	payload, err := json.Marshal(challenge)
	if err != nil {
		return err
	}
	return b.store.Client().Set(
		context.Background(),
		b.store.Key("anonymous", "challenge", challenge.ID),
		payload,
		sharedredis.TTLUntil(challenge.ExpiresAt),
	).Err()
}

func (b *redisChallengeBackend) Get(id string) (*Challenge, error) {
	raw, err := b.store.Client().Get(
		context.Background(),
		b.store.Key("anonymous", "challenge", id),
	).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var challenge Challenge
	if err := json.Unmarshal(raw, &challenge); err != nil {
		return nil, err
	}
	if time.Now().UTC().After(challenge.ExpiresAt) {
		_ = b.Delete(id)
		return nil, nil
	}
	return &challenge, nil
}

func (b *redisChallengeBackend) Delete(id string) error {
	return b.store.Client().Del(
		context.Background(),
		b.store.Key("anonymous", "challenge", id),
	).Err()
}

type redisNullifierBackend struct {
	store *sharedredis.Store
}

// NewRedisNullifierBackend creates a Redis-backed nullifier reservation store.
func NewRedisNullifierBackend(store *sharedredis.Store) nullifierStoreBackend {
	return &redisNullifierBackend{store: store}
}

func (b *redisNullifierBackend) Consume(nullifier string, ttl time.Duration) (bool, error) {
	if nullifier == "" {
		return false, nil
	}
	if ttl <= 0 {
		ttl = time.Minute
	}
	return b.store.Client().SetNX(
		context.Background(),
		b.store.Key("anonymous", "nullifier", nullifier),
		"1",
		ttl,
	).Result()
}

func (b *redisNullifierBackend) IsConsumed(nullifier string) (bool, error) {
	if nullifier == "" {
		return false, nil
	}
	count, err := b.store.Client().Exists(
		context.Background(),
		b.store.Key("anonymous", "nullifier", nullifier),
	).Result()
	return count > 0, err
}

func (b *redisNullifierBackend) Release(nullifier string) error {
	if nullifier == "" {
		return nil
	}
	return b.store.Client().Del(
		context.Background(),
		b.store.Key("anonymous", "nullifier", nullifier),
	).Err()
}
