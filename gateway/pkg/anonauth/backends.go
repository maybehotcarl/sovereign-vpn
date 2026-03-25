package anonauth

import "time"

type challengeStoreBackend interface {
	Set(challenge *Challenge) error
	Get(id string) (*Challenge, error)
	Delete(id string) error
}

type nullifierStoreBackend interface {
	Consume(nullifier string, ttl time.Duration) (bool, error)
	IsConsumed(nullifier string) (bool, error)
	Release(nullifier string) error
}

type inMemoryChallengeBackend struct {
	store *ChallengeStore
}

func newInMemoryChallengeBackend() challengeStoreBackend {
	return &inMemoryChallengeBackend{store: NewChallengeStore()}
}

func (b *inMemoryChallengeBackend) Set(challenge *Challenge) error {
	b.store.Set(challenge)
	return nil
}

func (b *inMemoryChallengeBackend) Get(id string) (*Challenge, error) {
	return b.store.Get(id), nil
}

func (b *inMemoryChallengeBackend) Delete(id string) error {
	b.store.Delete(id)
	return nil
}

type inMemoryNullifierBackend struct {
	store *NullifierStore
}

func newInMemoryNullifierBackend() nullifierStoreBackend {
	return &inMemoryNullifierBackend{store: NewNullifierStore()}
}

func (b *inMemoryNullifierBackend) Consume(nullifier string, ttl time.Duration) (bool, error) {
	return b.store.Consume(nullifier, ttl), nil
}

func (b *inMemoryNullifierBackend) IsConsumed(nullifier string) (bool, error) {
	return b.store.IsConsumed(nullifier), nil
}

func (b *inMemoryNullifierBackend) Release(nullifier string) error {
	b.store.Release(nullifier)
	return nil
}
