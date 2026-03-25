package siwe

type nonceStoreBackend interface {
	Generate(length int) (string, error)
	Consume(nonce string) (bool, error)
}

type inMemoryNonceBackend struct {
	store *NonceStore
}

func wrapNonceStore(store *NonceStore) nonceStoreBackend {
	return &inMemoryNonceBackend{store: store}
}

func (b *inMemoryNonceBackend) Generate(length int) (string, error) {
	return b.store.Generate(length)
}

func (b *inMemoryNonceBackend) Consume(nonce string) (bool, error) {
	return b.store.Consume(nonce), nil
}
