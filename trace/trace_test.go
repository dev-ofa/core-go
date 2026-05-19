package trace

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewTraceID(t *testing.T) {
	traceID, err := NewTraceID()

	require.NoError(t, err)
	require.Regexp(t, regexp.MustCompile(`^[0-9a-f]{32}$`), traceID)
}

func TestNewRequestIDWithTime(t *testing.T) {
	now := time.Date(2026, 4, 20, 15, 30, 45, 0, time.UTC)

	requestID, err := NewRequestIDWithTime(now)

	require.NoError(t, err)
	require.Regexp(t, regexp.MustCompile(`^req_20260420_153045_[a-z2-7]{16}$`), requestID)
}
