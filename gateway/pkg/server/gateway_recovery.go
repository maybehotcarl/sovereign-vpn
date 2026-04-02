package server

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/nftgate"
)

type gatewayOwnerState int

const (
	gatewayOwnerStateUnknown gatewayOwnerState = iota
	gatewayOwnerStateLive
	gatewayOwnerStateDead
)

func (s *Server) gatewayOwnerState(session *nftgate.Session) (gatewayOwnerState, error) {
	if session == nil || strings.TrimSpace(session.GatewayInstanceID) == "" {
		return gatewayOwnerStateUnknown, nil
	}
	if s.sessionOwnedByCurrentGateway(session) {
		return gatewayOwnerStateLive, nil
	}
	if s == nil || s.gatewayPresence == nil || !s.gatewayPresenceShared {
		return gatewayOwnerStateUnknown, nil
	}

	presence, err := s.gatewayPresence.Get(session.GatewayInstanceID)
	if err != nil {
		return gatewayOwnerStateUnknown, err
	}
	if presence == nil || !presence.ExpiresAt.After(time.Now().UTC()) {
		return gatewayOwnerStateDead, nil
	}
	return gatewayOwnerStateLive, nil
}

func (s *Server) clearSessionPeerState(sessionID string, gatewayInstanceID string) error {
	if s == nil || s.peerStates == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}

	states, err := s.peerStates.ListBySession(sessionID)
	if err != nil {
		return fmt.Errorf("listing peer state for dead-session recovery: %w", err)
	}

	var firstErr error
	for _, state := range states {
		if state == nil || strings.TrimSpace(state.PublicKey) == "" {
			continue
		}
		if gatewayInstanceID != "" && state.GatewayInstanceID != gatewayInstanceID {
			continue
		}
		if err := s.deletePeerOwner(state.PublicKey); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("releasing peer reservation during dead-session recovery: %w", err)
		}
		if err := s.deletePeerState(state.PublicKey); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("deleting peer state during dead-session recovery: %w", err)
		}
	}

	return firstErr
}

func (s *Server) clearDeadSessionOwnership(session *nftgate.Session) (*nftgate.Session, error) {
	if session == nil || strings.TrimSpace(session.GatewayInstanceID) == "" {
		return session, nil
	}

	oldOwner := session.GatewayInstanceID
	if err := s.clearSessionPeerState(session.ID, oldOwner); err != nil {
		return nil, err
	}
	if s != nil && s.peerOwners != nil {
		if err := s.peerOwners.ReleaseByOwner(session.ID, oldOwner); err != nil {
			return nil, fmt.Errorf("releasing stale peer reservations for dead gateway: %w", err)
		}
	}
	if err := s.gate.ReleaseSessionGateway(session.ID, oldOwner); err != nil {
		return nil, fmt.Errorf("releasing dead gateway owner: %w", err)
	}

	refreshed, err := s.gate.GetSessionByTokenWithError(session.Token)
	if err != nil {
		return nil, fmt.Errorf("reloading session after dead-owner cleanup: %w", err)
	}
	if refreshed == nil {
		return nil, nil
	}

	if refreshed.GatewayInstanceID == "" {
		log.Printf("Recovered dead gateway binding")
		return refreshed, nil
	}
	if refreshed.GatewayInstanceID != oldOwner {
		log.Printf("Observed concurrent dead gateway recovery")
		return refreshed, nil
	}

	log.Printf("Recovered dead gateway binding without releasing owner")
	return refreshed, nil
}

func (s *Server) takeoverDeadSession(session *nftgate.Session) (*nftgate.Session, bool, error) {
	reconciled, err := s.clearDeadSessionOwnership(session)
	if err != nil {
		return nil, false, err
	}
	if reconciled == nil {
		return nil, false, nil
	}
	if s.sessionOwnedByCurrentGateway(reconciled) {
		return reconciled, false, nil
	}
	if strings.TrimSpace(reconciled.GatewayInstanceID) != "" {
		return reconciled, false, nil
	}

	rebound, newlyBound, err := s.gate.BindSessionGateway(reconciled.ID, s.currentGatewayIdentity())
	if err != nil {
		return nil, false, fmt.Errorf("binding recovered session to current gateway: %w", err)
	}
	if rebound != nil && s.sessionOwnedByCurrentGateway(rebound) {
		log.Printf("Took over dead gateway session")
	}
	return rebound, newlyBound, nil
}
