// Package packer batches sealed records into slab-sized flushes.
package packer

import "time"

// A Policy decides when a queue of records becomes a flush.
//
// Every flush mints a brand-new slab, billed whole and never extendable, so the
// policy is a slab budget rather than a latency knob: flushing twice as often
// costs twice the quota for the same data.
type Policy struct {
	// IdleAfter flushes once no record has arrived for this long.
	IdleAfter time.Duration
	// MaxAge bounds how long a record can exist only on this device. It is a
	// durability control, not a cost control.
	MaxAge time.Duration
	// MaxBytes flushes when the queue would no longer fit one slab. At
	// realistic record sizes this fires on the order of years, so it is a
	// bulk-import valve rather than the normal trigger.
	MaxBytes int64
	// MaxRecords bounds the queue so a burst cannot grow it without limit.
	MaxRecords int
}

// DefaultPolicy is the settled cadence: flush after a short idle period, and in
// any case within the hour.
func DefaultPolicy(slabBytes int64) Policy {
	return Policy{
		IdleAfter:  5 * time.Minute,
		MaxAge:     time.Hour,
		MaxBytes:   slabBytes,
		MaxRecords: 100_000,
	}
}

// Immediate flushes after every record.
//
// It is the most expensive setting there is, one slab per record, and exists
// so an end-to-end path can be exercised without waiting on a timer, not as
// something to run a vault on.
func Immediate(slabBytes int64) Policy {
	return Policy{MaxBytes: slabBytes, MaxRecords: 1}
}

// A Reason names why a flush fired, so the cost of a cadence can be attributed
// rather than guessed at.
type Reason string

const (
	ReasonIdle    Reason = "idle"
	ReasonAge     Reason = "age"
	ReasonBytes   Reason = "bytes"
	ReasonRecords Reason = "records"
	ReasonManual  Reason = "manual"
)

// due reports whether a queue in this state should flush, and why.
func (p Policy) due(records int, bytes int64, oldest, latest time.Time, now time.Time) (Reason, bool) {
	if records == 0 {
		return "", false
	}
	if p.MaxRecords > 0 && records >= p.MaxRecords {
		return ReasonRecords, true
	}
	if p.MaxBytes > 0 && bytes >= p.MaxBytes {
		return ReasonBytes, true
	}
	if p.MaxAge > 0 && now.Sub(oldest) >= p.MaxAge {
		return ReasonAge, true
	}
	if p.IdleAfter > 0 && now.Sub(latest) >= p.IdleAfter {
		return ReasonIdle, true
	}
	return "", false
}
