package server

import (
	"fmt"
	"strconv"

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
	PolicyEpoch      string
	EntitlementClass string
	ExpiryBucket     string
	NullifierHash    string
	ChallengeHash    string
	SessionKeyHash   string
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

	signals := &vpnAccessV1Signals{
		Root:             req.PublicSignals[vpnAccessRootIndex],
		PolicyEpoch:      req.PublicSignals[vpnAccessEpochIndex],
		EntitlementClass: req.PublicSignals[vpnAccessClassIndex],
		ExpiryBucket:     req.PublicSignals[vpnAccessExpiryIndex],
		NullifierHash:    req.PublicSignals[vpnAccessNullifierIndex],
		ChallengeHash:    req.PublicSignals[vpnAccessChallengeIndex],
		SessionKeyHash:   req.PublicSignals[vpnAccessSessionIndex],
	}

	if signals.Root == "" || signals.PolicyEpoch == "" || signals.EntitlementClass == "" ||
		signals.ExpiryBucket == "" || signals.NullifierHash == "" || signals.ChallengeHash == "" ||
		signals.SessionKeyHash == "" {
		return nil, fmt.Errorf("vpn_access_v1 public signals must all be non-empty")
	}

	if signals.PolicyEpoch != strconv.FormatUint(challenge.PolicyEpoch, 10) {
		return nil, fmt.Errorf("vpn_access_v1 policy_epoch does not match challenge")
	}
	if signals.NullifierHash != req.NullifierHash {
		return nil, fmt.Errorf("vpn_access_v1 nullifier_hash does not match request")
	}
	if signals.SessionKeyHash != req.SessionKeyHash {
		return nil, fmt.Errorf("vpn_access_v1 session_key_hash does not match request")
	}

	return signals, nil
}
