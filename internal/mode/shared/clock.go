// Package shared provides common types and utilities for mode controllers.
package shared

import (
	"fmt"
	"time"
)

// Clock provides the current time. Use RealClock for production
// and mocks.MockClock for testing.
type Clock interface {
	Now() time.Time
}

// RealClock returns the actual current time.
type RealClock struct{}

// Now returns the current time.
func (RealClock) Now() time.Time { return time.Now() }

// FormatRelativeTimeWithClock returns a human-friendly relative timestamp using the provided clock.
// Examples: "now", "5m ago", "3h ago", "2d ago", "1w ago", "3mo ago", "1y ago"
func FormatRelativeTimeWithClock(t time.Time, clock Clock) string {
	return FormatRelativeTimeFrom(t, clock.Now())
}

// FormatRelativeTimeFrom returns a human-friendly relative timestamp
// relative to the given reference time. This is useful for testing.
func FormatRelativeTimeFrom(t, now time.Time) string {
	d := now.Sub(t)

	// Handle negative duration (future timestamps)
	if d < 0 {
		return "now"
	}

	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 4*7*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(d.Hours()/(24*7)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy ago", int(d.Hours()/(24*365)))
	}
}
