package nftcheck

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
)

// AccessChecker checks whether a wallet has VPN access.
// Implemented by both Checker (AccessPolicy mode) and DirectChecker (direct ERC-1155 mode).
type AccessChecker interface {
	Check(ctx context.Context, wallet common.Address) (CheckResult, error)
	Invalidate(wallet common.Address)
	Close()
}
