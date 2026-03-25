package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/nftgate"
)

const (
	internalForwardConnectPath    = "/internal/forward/vpn/connect"
	internalForwardDisconnectPath = "/internal/forward/vpn/disconnect"
	internalForwardStatusPath     = "/internal/forward/vpn/status"

	forwardHeaderBy        = "X-Sovereign-Forwarded-By"
	forwardHeaderAt        = "X-Sovereign-Forwarded-At"
	forwardHeaderSignature = "X-Sovereign-Forwarded-Signature"

	internalForwardMaxSkew = 1 * time.Minute
)

type internalForwardContextKey string

const forwardedRequestContextKey internalForwardContextKey = "sovereign-vpn-internal-forward"

func (s *Server) forwardingEnabled() bool {
	return s != nil && s.cfg != nil && strings.TrimSpace(s.cfg.GatewayForwardingKey) != ""
}

func (s *Server) tryForwardConnectToOwner(
	w http.ResponseWriter,
	r *http.Request,
	session *nftgate.Session,
	req ConnectRequest,
) bool {
	body, err := json.Marshal(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to forward connect request")
		return true
	}
	return s.tryForwardToOwner(w, r, session, http.MethodPost, internalForwardConnectPath, body)
}

func (s *Server) tryForwardDisconnectToOwner(
	w http.ResponseWriter,
	r *http.Request,
	session *nftgate.Session,
	req ConnectRequest,
) bool {
	body, err := json.Marshal(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to forward disconnect request")
		return true
	}
	return s.tryForwardToOwner(w, r, session, http.MethodPost, internalForwardDisconnectPath, body)
}

func (s *Server) tryForwardStatusToOwner(
	w http.ResponseWriter,
	r *http.Request,
	session *nftgate.Session,
) bool {
	return s.tryForwardToOwner(w, r, session, http.MethodGet, internalForwardStatusPath, nil)
}

func (s *Server) tryForwardToOwner(
	w http.ResponseWriter,
	r *http.Request,
	session *nftgate.Session,
	method string,
	internalPath string,
	body []byte,
) bool {
	if isInternalForwardedRequest(r.Context()) {
		return false
	}
	if session == nil || strings.TrimSpace(s.ownerForwardBaseURL(session)) == "" || !s.forwardingEnabled() {
		return false
	}

	resp, err := s.forwardToOwner(r.Context(), session, method, internalPath, body, r.Header.Get("Authorization"))
	if err != nil {
		writeError(w, http.StatusBadGateway, "owner gateway forwarding failed")
		return true
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
	return true
}

func (s *Server) forwardToOwner(
	ctx context.Context,
	session *nftgate.Session,
	method string,
	internalPath string,
	body []byte,
	authorization string,
) (*http.Response, error) {
	targetBaseURL := s.ownerForwardBaseURL(session)
	if session == nil || strings.TrimSpace(targetBaseURL) == "" {
		return nil, fmt.Errorf("owner gateway URL unavailable")
	}
	if s.forwardHTTPClient == nil {
		s.forwardHTTPClient = &http.Client{Timeout: 5 * time.Second}
	}

	targetURL := targetBaseURL + internalPath
	req, err := http.NewRequestWithContext(ctx, method, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(authorization) != "" {
		req.Header.Set("Authorization", authorization)
	}

	forwardedAt := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	req.Header.Set(forwardHeaderBy, s.currentGatewayInstanceID())
	req.Header.Set(forwardHeaderAt, forwardedAt)
	req.Header.Set(
		forwardHeaderSignature,
		s.signForwardedRequest(method, internalPath, body, s.currentGatewayInstanceID(), forwardedAt),
	)

	return s.forwardHTTPClient.Do(req)
}

func (s *Server) ownerForwardBaseURL(session *nftgate.Session) string {
	if session == nil {
		return ""
	}
	if forwardURL := strings.TrimRight(strings.TrimSpace(session.GatewayForwardURL), "/"); forwardURL != "" {
		return forwardURL
	}
	return strings.TrimRight(strings.TrimSpace(session.GatewayPublicURL), "/")
}

func (s *Server) signForwardedRequest(
	method string,
	path string,
	body []byte,
	forwardedBy string,
	forwardedAt string,
) string {
	payload := strings.Join([]string{
		method,
		path,
		forwardedBy,
		forwardedAt,
		hashForwardBody(body),
	}, "\n")

	mac := hmac.New(sha256.New, []byte(s.cfg.GatewayForwardingKey))
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func hashForwardBody(body []byte) string {
	sum := sha256.Sum256(body)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (s *Server) verifyForwardedRequest(r *http.Request, body []byte) error {
	if !s.forwardingEnabled() {
		return fmt.Errorf("gateway forwarding is not configured")
	}

	forwardedBy := strings.TrimSpace(r.Header.Get(forwardHeaderBy))
	forwardedAt := strings.TrimSpace(r.Header.Get(forwardHeaderAt))
	signature := strings.TrimSpace(r.Header.Get(forwardHeaderSignature))
	if forwardedBy == "" || forwardedAt == "" || signature == "" {
		return fmt.Errorf("missing forwarded request headers")
	}

	timestamp, err := strconv.ParseInt(forwardedAt, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid forwarded timestamp")
	}

	now := time.Now().UTC()
	signedAt := time.Unix(timestamp, 0).UTC()
	if signedAt.Before(now.Add(-internalForwardMaxSkew)) || signedAt.After(now.Add(internalForwardMaxSkew)) {
		return fmt.Errorf("forwarded request timestamp out of range")
	}

	expected := s.signForwardedRequest(r.Method, r.URL.Path, body, forwardedBy, forwardedAt)
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return fmt.Errorf("invalid forwarded request signature")
	}

	return nil
}

func isInternalForwardedRequest(ctx context.Context) bool {
	forwarded, _ := ctx.Value(forwardedRequestContextKey).(bool)
	return forwarded
}

func withInternalForwardedRequest(ctx context.Context) context.Context {
	return context.WithValue(ctx, forwardedRequestContextKey, true)
}

func (s *Server) handleInternalForwardVPNConnect(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read forwarded request body")
		return
	}
	if err := s.verifyForwardedRequest(r, body); err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	req, err := http.NewRequestWithContext(
		withInternalForwardedRequest(r.Context()),
		http.MethodPost,
		"/vpn/connect",
		bytes.NewReader(body),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to process forwarded connect request")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	s.handleVPNConnect(w, req)
}

func (s *Server) handleInternalForwardVPNDisconnect(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read forwarded request body")
		return
	}
	if err := s.verifyForwardedRequest(r, body); err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	req, err := http.NewRequestWithContext(
		withInternalForwardedRequest(r.Context()),
		http.MethodPost,
		"/vpn/disconnect",
		bytes.NewReader(body),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to process forwarded disconnect request")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	s.handleVPNDisconnect(w, req)
}

func (s *Server) handleInternalForwardVPNStatus(w http.ResponseWriter, r *http.Request) {
	if err := s.verifyForwardedRequest(r, nil); err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	req := r.Clone(withInternalForwardedRequest(r.Context()))
	req.URL.Path = "/vpn/status"
	s.handleVPNStatus(w, req)
}
