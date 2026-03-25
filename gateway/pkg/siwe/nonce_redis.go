package siwe

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/sharedredis"
)

type redisNonceBackend struct {
	store *sharedredis.Store
	ttl   time.Duration
}

// NewRedisNonceBackend creates a Redis-backed SIWE nonce store.
func NewRedisNonceBackend(store *sharedredis.Store, ttl time.Duration) nonceStoreBackend {
	return &redisNonceBackend{
		store: store,
		ttl:   ttl,
	}
}

func (b *redisNonceBackend) Generate(length int) (string, error) {
	if length <= 0 {
		length = 16
	}

	for attempt := 0; attempt < 5; attempt++ {
		bytes := make([]byte, length)
		if _, err := rand.Read(bytes); err != nil {
			return "", err
		}
		nonce := hex.EncodeToString(bytes)
		ok, err := b.store.Client().SetNX(
			context.Background(),
			b.store.Key("siwe", "nonce", nonce),
			"1",
			b.ttl,
		).Result()
		if err != nil {
			return "", err
		}
		if ok {
			return nonce, nil
		}
	}

	return "", redis.TxFailedErr
}

func (b *redisNonceBackend) Consume(nonce string) (bool, error) {
	_, err := b.store.Client().GetDel(
		context.Background(),
		b.store.Key("siwe", "nonce", nonce),
	).Result()
	if err == redis.Nil {
		return false, nil
	}
	return err == nil, err
}
