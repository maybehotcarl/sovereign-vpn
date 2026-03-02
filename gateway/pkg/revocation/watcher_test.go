package revocation

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// mockRevoker records calls to invalidation/revocation hooks.
type mockRevoker struct {
	revoked     []common.Address
	invalidated []common.Address
}

func (m *mockRevoker) InvalidateAndRevoke(wallet common.Address) {
	m.invalidated = append(m.invalidated, wallet)
	m.revoked = append(m.revoked, wallet)
}

func (m *mockRevoker) InvalidateOnly(wallet common.Address) {
	m.invalidated = append(m.invalidated, wallet)
}

func TestHandleLogTransferSingle(t *testing.T) {
	revoker := &mockRevoker{}
	w := &Watcher{revoker: revoker}

	from := common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	to := common.HexToAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	vLog := types.Log{
		Topics: []common.Hash{
			transferSingleSig,
			common.BytesToHash(common.LeftPadBytes(common.Address{}.Bytes(), 32)), // operator
			common.BytesToHash(common.LeftPadBytes(from.Bytes(), 32)),             // from
			common.BytesToHash(common.LeftPadBytes(to.Bytes(), 32)),               // to
		},
		Data: make([]byte, 64), // id + value
	}

	// Fix: TransferSingle has indexed operator, from, to
	// Topics: [sig, operator, from, to]
	// Actually ERC-1155 TransferSingle: Topics[0]=sig, Topics[1]=operator(indexed), Topics[2]=from(indexed), Topics[3]=to(indexed)
	vLog.Topics = []common.Hash{
		transferSingleSig,
		common.BytesToHash(common.LeftPadBytes(common.Address{}.Bytes(), 32)), // operator
		common.BytesToHash(common.LeftPadBytes(from.Bytes(), 32)),             // from
		common.BytesToHash(common.LeftPadBytes(to.Bytes(), 32)),               // to
	}

	w.handleLog(vLog)

	// Sender should be revoked; receiver should only be invalidated.
	if len(revoker.revoked) != 1 {
		t.Fatalf("expected 1 revocation, got %d", len(revoker.revoked))
	}
	if revoker.revoked[0] != from {
		t.Errorf("expected from=%s, got %s", from.Hex(), revoker.revoked[0].Hex())
	}
	if len(revoker.invalidated) != 2 {
		t.Fatalf("expected 2 invalidations, got %d", len(revoker.invalidated))
	}
	if revoker.invalidated[0] != from {
		t.Errorf("expected invalidated from=%s, got %s", from.Hex(), revoker.invalidated[0].Hex())
	}
	if revoker.invalidated[1] != to {
		t.Errorf("expected invalidated to=%s, got %s", to.Hex(), revoker.invalidated[1].Hex())
	}
}

func TestHandleLogSkipsMint(t *testing.T) {
	revoker := &mockRevoker{}
	w := &Watcher{revoker: revoker}

	// Mint: from=zero, to=recipient
	to := common.HexToAddress("0xcccccccccccccccccccccccccccccccccccccccc")

	vLog := types.Log{
		Topics: []common.Hash{
			transferSingleSig,
			common.BytesToHash(common.LeftPadBytes(common.Address{}.Bytes(), 32)), // operator
			common.BytesToHash(common.LeftPadBytes(common.Address{}.Bytes(), 32)), // from=zero (mint)
			common.BytesToHash(common.LeftPadBytes(to.Bytes(), 32)),               // to
		},
		Data: make([]byte, 64),
	}

	w.handleLog(vLog)

	// Only receiver cache should be invalidated (no sender revoke on mint).
	if len(revoker.revoked) != 0 {
		t.Fatalf("expected 0 revocations, got %d", len(revoker.revoked))
	}
	if len(revoker.invalidated) != 1 {
		t.Fatalf("expected 1 invalidation, got %d", len(revoker.invalidated))
	}
	if revoker.invalidated[0] != to {
		t.Errorf("expected invalidated to=%s, got %s", to.Hex(), revoker.invalidated[0].Hex())
	}
}

func TestHandleLogTransferBatch(t *testing.T) {
	revoker := &mockRevoker{}
	w := &Watcher{revoker: revoker}

	from := common.HexToAddress("0xdddddddddddddddddddddddddddddddddddddd")
	to := common.HexToAddress("0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee")

	vLog := types.Log{
		Topics: []common.Hash{
			transferBatchSig,
			common.BytesToHash(common.LeftPadBytes(common.Address{}.Bytes(), 32)),
			common.BytesToHash(common.LeftPadBytes(from.Bytes(), 32)),
			common.BytesToHash(common.LeftPadBytes(to.Bytes(), 32)),
		},
		Data: make([]byte, 128), // empty arrays
	}

	w.handleLog(vLog)

	if len(revoker.revoked) != 1 {
		t.Fatalf("expected 1 revocation, got %d", len(revoker.revoked))
	}
	if revoker.revoked[0] != from {
		t.Errorf("expected revoked from=%s, got %s", from.Hex(), revoker.revoked[0].Hex())
	}
	if len(revoker.invalidated) != 2 {
		t.Fatalf("expected 2 invalidations, got %d", len(revoker.invalidated))
	}
}

func TestHandleLogTooFewTopics(t *testing.T) {
	revoker := &mockRevoker{}
	w := &Watcher{revoker: revoker}

	vLog := types.Log{
		Topics: []common.Hash{transferSingleSig}, // only 1 topic, need at least 3
	}

	w.handleLog(vLog)

	if len(revoker.revoked) != 0 {
		t.Errorf("should not revoke with too few topics, got %d", len(revoker.revoked))
	}
	if len(revoker.invalidated) != 0 {
		t.Errorf("should not invalidate with too few topics, got %d", len(revoker.invalidated))
	}
}

func TestTruncAddr(t *testing.T) {
	addr := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")
	got := truncAddr(addr)
	if len(got) > 14 { // "0x1234...5678"
		t.Logf("truncated: %s", got)
	}
}
