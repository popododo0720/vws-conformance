package auth

import (
	"testing"
	"time"
)

func TestSkewPastS3Window(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	got := skewedSigningTime(now)
	limit := now.Add(-15 * time.Minute)
	if !got.Before(limit) {
		t.Fatalf("signing time %s is inside the 15 minute S3 window ending %s", got, limit)
	}
}
