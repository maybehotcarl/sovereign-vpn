package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/maybehotcarl/sovereign-vpn/gateway/pkg/siwe"
)

const (
	operatorEnrollmentTTL      = 24 * time.Hour
	operatorEnrollmentTokenLen = 16
)

var nodeRegionPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

type operatorEnrollmentStore struct {
	mu      sync.RWMutex
	ttl     time.Duration
	records map[string]*OperatorEnrollment
	now     func() time.Time
}

type OperatorEnrollment struct {
	Token        string                    `json:"token"`
	Operator     string                    `json:"operator"`
	Region       string                    `json:"region"`
	Status       string                    `json:"status"`
	CreatedAt    time.Time                 `json:"created_at"`
	ExpiresAt    time.Time                 `json:"expires_at"`
	LastReportAt *time.Time                `json:"last_report_at,omitempty"`
	Report       *OperatorEnrollmentReport `json:"report,omitempty"`
}

type OperatorEnrollmentReport struct {
	Operator         string    `json:"operator,omitempty"`
	Region           string    `json:"region,omitempty"`
	Endpoint         string    `json:"endpoint,omitempty"`
	GatewayURL       string    `json:"gateway_url,omitempty"`
	PublicIP         string    `json:"public_ip,omitempty"`
	GatewayPort      string    `json:"gateway_port,omitempty"`
	WireGuardPort    string    `json:"wireguard_port,omitempty"`
	WireGuardPubKey  string    `json:"wireguard_public_key,omitempty"`
	HealthOK         bool      `json:"health_ok"`
	HealthStatus     string    `json:"health_status,omitempty"`
	InstallerVersion string    `json:"installer_version,omitempty"`
	ReportedAt       time.Time `json:"reported_at"`
}

type createOperatorEnrollmentRequest struct {
	Operator  string `json:"operator"`
	Region    string `json:"region"`
	Message   string `json:"message"`
	Signature string `json:"signature"`
}

func newOperatorEnrollmentStore(ttl time.Duration) *operatorEnrollmentStore {
	return &operatorEnrollmentStore{
		ttl:     ttl,
		records: make(map[string]*OperatorEnrollment),
		now:     time.Now,
	}
}

func (s *operatorEnrollmentStore) create(operator string, region string) (*OperatorEnrollment, error) {
	operator, err := normalizeEnrollmentOperator(operator)
	if err != nil {
		return nil, err
	}
	region, err = normalizeEnrollmentRegion(region)
	if err != nil {
		return nil, err
	}

	token, err := randomEnrollmentToken()
	if err != nil {
		return nil, err
	}

	now := s.now().UTC()
	record := &OperatorEnrollment{
		Token:     token,
		Operator:  operator,
		Region:    region,
		Status:    "created",
		CreatedAt: now,
		ExpiresAt: now.Add(s.ttl),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked(now)
	s.records[token] = record
	return cloneOperatorEnrollment(record), nil
}

func (s *operatorEnrollmentStore) get(token string) (*OperatorEnrollment, bool) {
	token = strings.TrimSpace(token)
	now := s.now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked(now)

	record, ok := s.records[token]
	if !ok {
		return nil, false
	}
	if !record.ExpiresAt.After(now) {
		delete(s.records, token)
		return nil, false
	}
	return cloneOperatorEnrollment(record), true
}

func (s *operatorEnrollmentStore) report(token string, report OperatorEnrollmentReport) (*OperatorEnrollment, error) {
	token = strings.TrimSpace(token)
	now := s.now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked(now)

	record, ok := s.records[token]
	if !ok || !record.ExpiresAt.After(now) {
		return nil, fmt.Errorf("enrollment not found")
	}

	if report.Operator != "" {
		operator, err := normalizeEnrollmentOperator(report.Operator)
		if err != nil {
			return nil, err
		}
		if !strings.EqualFold(operator, record.Operator) {
			return nil, fmt.Errorf("operator does not match enrollment")
		}
		report.Operator = operator
	}

	if report.Region != "" {
		region, err := normalizeEnrollmentRegion(report.Region)
		if err != nil {
			return nil, err
		}
		report.Region = region
	} else {
		report.Region = record.Region
	}

	report.Endpoint = truncateEnrollmentField(report.Endpoint, 256)
	report.GatewayURL = truncateEnrollmentField(report.GatewayURL, 256)
	report.PublicIP = truncateEnrollmentField(report.PublicIP, 128)
	report.GatewayPort = truncateEnrollmentField(report.GatewayPort, 16)
	report.WireGuardPort = truncateEnrollmentField(report.WireGuardPort, 16)
	report.WireGuardPubKey = truncateEnrollmentField(report.WireGuardPubKey, 128)
	report.HealthStatus = truncateEnrollmentField(report.HealthStatus, 64)
	report.InstallerVersion = truncateEnrollmentField(report.InstallerVersion, 64)
	report.ReportedAt = now

	record.Report = &report
	record.LastReportAt = &now
	if report.HealthOK {
		record.Status = "healthy"
	} else {
		record.Status = "reported"
	}

	return cloneOperatorEnrollment(record), nil
}

func (s *operatorEnrollmentStore) cleanupLocked(now time.Time) {
	for token, record := range s.records {
		if !record.ExpiresAt.After(now) {
			delete(s.records, token)
		}
	}
}

func (s *Server) handleCreateOperatorEnrollment(w http.ResponseWriter, r *http.Request) {
	var req createOperatorEnrollmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	operator, err := normalizeEnrollmentOperator(req.Operator)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Message == "" || req.Signature == "" {
		writeError(w, http.StatusBadRequest, "message and signature are required")
		return
	}
	if s.siwe == nil {
		writeError(w, http.StatusServiceUnavailable, "operator authentication not configured")
		return
	}

	auth, err := s.siwe.Verify(&siwe.SignedMessage{
		Message:   req.Message,
		Signature: req.Signature,
	})
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	if !strings.EqualFold(auth.Address.Hex(), operator) {
		writeError(w, http.StatusUnauthorized, "signed operator does not match request")
		return
	}

	enrollment, err := s.enrollments.create(operator, req.Region)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, enrollment)
}

func (s *Server) handleGetOperatorEnrollment(w http.ResponseWriter, r *http.Request) {
	token := tokenFromEnrollmentPath(r.URL.Path)
	if token == "" {
		writeError(w, http.StatusBadRequest, "enrollment token is required")
		return
	}

	enrollment, ok := s.enrollments.get(token)
	if !ok {
		writeError(w, http.StatusNotFound, "enrollment not found")
		return
	}

	writeJSON(w, http.StatusOK, enrollment)
}

func (s *Server) handleReportOperatorEnrollment(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(strings.Trim(r.URL.Path, "/"), "/report") {
		writeError(w, http.StatusNotFound, "enrollment report endpoint not found")
		return
	}

	token := strings.TrimSuffix(tokenFromEnrollmentPath(r.URL.Path), "/report")
	if token == "" {
		writeError(w, http.StatusBadRequest, "enrollment token is required")
		return
	}

	var report OperatorEnrollmentReport
	if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	enrollment, err := s.enrollments.report(token, report)
	if err != nil {
		if err.Error() == "enrollment not found" {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, enrollment)
}

func tokenFromEnrollmentPath(path string) string {
	token := strings.TrimPrefix(path, "/operator/enrollments/")
	return strings.Trim(token, "/")
}

func normalizeEnrollmentOperator(operator string) (string, error) {
	operator = strings.TrimSpace(operator)
	if !common.IsHexAddress(operator) {
		return "", fmt.Errorf("operator must be a valid Ethereum address")
	}
	return common.HexToAddress(operator).Hex(), nil
}

func normalizeEnrollmentRegion(region string) (string, error) {
	region = strings.ToLower(strings.TrimSpace(region))
	if region == "" {
		return "", fmt.Errorf("region is required")
	}
	if !nodeRegionPattern.MatchString(region) {
		return "", fmt.Errorf("region must use lowercase letters, numbers, and dashes")
	}
	return region, nil
}

func randomEnrollmentToken() (string, error) {
	b := make([]byte, operatorEnrollmentTokenLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func truncateEnrollmentField(value string, maxLen int) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen]
}

func cloneOperatorEnrollment(record *OperatorEnrollment) *OperatorEnrollment {
	clone := *record
	if record.LastReportAt != nil {
		t := *record.LastReportAt
		clone.LastReportAt = &t
	}
	if record.Report != nil {
		report := *record.Report
		clone.Report = &report
	}
	return &clone
}
