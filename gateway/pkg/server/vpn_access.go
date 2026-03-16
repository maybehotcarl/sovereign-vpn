package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/anonauth"
)

const (
	vpnAccessV1ProofType    = "vpn_access_v1"
	vpnAccessV1SignalCount  = 7
	vpnAccessRootIndex      = 0
	vpnAccessEpochIndex     = 1
	vpnAccessClassIndex     = 2
	vpnAccessExpiryIndex    = 3
	vpnAccessNullifierIndex = 4
	vpnAccessChallengeIndex = 5
	vpnAccessSessionIndex   = 6
)

type vpnAccessV1Signals struct {
	Root             string
	PolicyEpoch      uint64
	EntitlementClass uint64
	ExpiryBucket     time.Time
	NullifierHash    string
	ChallengeHash    string
	SessionKeyHash   string
}

func deriveVPNAccessV1ChallengeHash(challenge *anonauth.Challenge) string {
	payload := strings.Join([]string{
		vpnAccessV1ProofType,
		challenge.ID,
		challenge.Nonce,
		strconv.FormatUint(challenge.PolicyEpoch, 10),
		strconv.FormatInt(challenge.ExpiresAt.UTC().Unix(), 10),
	}, "|")

	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func deriveVPNAccessV1SessionKeyHash(publicKey string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(publicKey)))
	return hex.EncodeToString(sum[:])
}

func validateVPNAccessV1Signals(
	challenge *anonauth.Challenge,
	req AnonymousConnectRequest,
) (*vpnAccessV1Signals, error) {
	if challenge == nil {
		return nil, fmt.Errorf("challenge required")
	}
	if len(req.PublicSignals) != vpnAccessV1SignalCount {
		return nil, fmt.Errorf("vpn_access_v1 requires exactly %d public signals", vpnAccessV1SignalCount)
	}

	policyEpoch, err := strconv.ParseUint(req.PublicSignals[vpnAccessEpochIndex], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("vpn_access_v1 policy_epoch must be an unsigned integer")
	}
	entitlementClass, err := strconv.ParseUint(req.PublicSignals[vpnAccessClassIndex], 10, 64)
	if err != nil || entitlementClass == 0 {
		return nil, fmt.Errorf("vpn_access_v1 entitlement_class must be a positive integer")
	}
	expiryUnix, err := strconv.ParseInt(req.PublicSignals[vpnAccessExpiryIndex], 10, 64)
	if err != nil || expiryUnix <= 0 {
		return nil, fmt.Errorf("vpn_access_v1 expiry_bucket must be a positive unix timestamp")
	}

	signals := &vpnAccessV1Signals{
		Root:             req.PublicSignals[vpnAccessRootIndex],
		PolicyEpoch:      policyEpoch,
		EntitlementClass: entitlementClass,
		ExpiryBucket:     time.Unix(expiryUnix, 0).UTC(),
		NullifierHash:    req.PublicSignals[vpnAccessNullifierIndex],
		ChallengeHash:    req.PublicSignals[vpnAccessChallengeIndex],
		SessionKeyHash:   req.PublicSignals[vpnAccessSessionIndex],
	}

	if signals.Root == "" ||
		req.PublicSignals[vpnAccessExpiryIndex] == "" || signals.NullifierHash == "" || signals.ChallengeHash == "" ||
		signals.SessionKeyHash == "" {
		return nil, fmt.Errorf("vpn_access_v1 public signals must all be non-empty")
	}

	if signals.PolicyEpoch != challenge.PolicyEpoch {
		return nil, fmt.Errorf("vpn_access_v1 policy_epoch does not match challenge")
	}
	if !signals.ExpiryBucket.After(time.Now().UTC()) {
		return nil, fmt.Errorf("vpn_access_v1 entitlement is already expired")
	}
	if signals.NullifierHash != req.NullifierHash {
		return nil, fmt.Errorf("vpn_access_v1 nullifier_hash does not match request")
	}
	expectedChallengeHash := deriveVPNAccessV1ChallengeHash(challenge)
	if signals.ChallengeHash != expectedChallengeHash {
		return nil, fmt.Errorf("vpn_access_v1 challenge_hash does not match challenge")
	}
	expectedSessionKeyHash := deriveVPNAccessV1SessionKeyHash(req.PublicKey)
	if req.SessionKeyHash != expectedSessionKeyHash {
		return nil, fmt.Errorf("vpn_access_v1 session_key_hash does not match public_key")
	}
	if signals.SessionKeyHash != expectedSessionKeyHash {
		return nil, fmt.Errorf("vpn_access_v1 session_key_hash does not match request")
	}

	return signals, nil
}
