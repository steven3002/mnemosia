package record

import (
	"fmt"
	"strings"
	"time"
)

// timeLayout is RFC 3339 in UTC at millisecond precision.
const timeLayout = "2006-01-02T15:04:05.000Z"

// A Time is a record timestamp.
//
// Timestamps are for display and for bi-temporal reasoning only. Ordering and
// causality use version counters, never the wall clock, so a device with a
// skewed clock cannot reorder history.
type Time struct{ time.Time }

// Now returns the current time at the schema's precision.
func Now() Time { return Time{time.Now().UTC().Truncate(time.Millisecond)} }

// At converts a wall-clock time to the schema's precision.
func At(t time.Time) Time { return Time{t.UTC().Truncate(time.Millisecond)} }

func (t Time) String() string { return t.UTC().Format(timeLayout) }

func (t Time) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.String() + `"`), nil
}

func (t *Time) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		t.Time = time.Time{}
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return fmt.Errorf("parse timestamp %q: %w", s, err)
	}
	t.Time = parsed.UTC().Truncate(time.Millisecond)
	return nil
}
