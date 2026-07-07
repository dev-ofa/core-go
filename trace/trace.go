package trace

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	// HeaderTraceID is the full-link trace header defined by the tracing spec.
	HeaderTraceID = "ofa-pass-trace-id"
	// HeaderOperator is the full-link operator header defined by the tracing spec.
	HeaderOperator = "ofa-pass-operator"
	// HeaderTenantID is the full-link tenant header defined by the tracing spec.
	HeaderTenantID = "ofa-pass-tenant-id"
	// HeaderAppID is the full-link application header defined by the tracing spec.
	HeaderAppID = "ofa-pass-app-id"
	// HeaderLocale is the full-link locale header defined by the i18n spec.
	HeaderLocale = "ofa-pass-locale"
	// HeaderRequestID is the single-hop request header defined by the tracing spec.
	HeaderRequestID = "ofa-direct-request-id"
	// HeaderRemainingTimeoutMS is the single-hop timeout budget header.
	HeaderRemainingTimeoutMS = "ofa-direct-remaining-timeout-ms"
)

// NewTraceID returns a 32-character lower-case hex trace id.
func NewTraceID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate trace id failed: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// NewRequestID returns a single-hop request id using the current UTC time.
func NewRequestID() (string, error) {
	return NewRequestIDWithTime(time.Now().UTC())
}

// NewRequestIDWithTime returns a single-hop request id using the provided time.
func NewRequestIDWithTime(now time.Time) (string, error) {
	buf := make([]byte, 10)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate request id failed: %w", err)
	}
	suffix := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)
	return fmt.Sprintf("req_%s_%s", now.Format("20060102_150405"), strings.ToLower(suffix)), nil
}
