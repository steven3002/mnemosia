package local

import (
	"fmt"
	"time"
)

// A TierStat is how one level of the read hierarchy has performed.
type TierStat struct {
	Tier  string
	Reads int64
	// Spent is the total time reads served by this tier took.
	Spent time.Duration
	// Misses counts times this tier was asked and could not answer, so the hit
	// rate of a level is measurable and not merely its share of traffic.
	Misses int64
}

// Mean is the average time a read served by this tier took.
func (t TierStat) Mean() time.Duration {
	if t.Reads == 0 {
		return 0
	}
	return t.Spent / time.Duration(t.Reads)
}

// RecordRead counts one read against the tier that served it.
//
// Hit rates are a property of how a vault is actually used, so they cannot be
// reconstructed later from anything the vault stores; they have to be counted
// as reads happen. Counting them is cheap and the alternative is having no
// answer at the point the answer is wanted.
func (s *Store) RecordRead(tier string, took time.Duration) error {
	_, err := s.db.Exec(
		`INSERT INTO read_stats (tier, reads, nanos) VALUES (?, 1, ?)
		 ON CONFLICT(tier) DO UPDATE SET reads = read_stats.reads + 1, nanos = read_stats.nanos + excluded.nanos`,
		tier, took.Nanoseconds())
	if err != nil {
		return fmt.Errorf("count a %s read: %w", tier, err)
	}
	return nil
}

// RecordMiss counts a tier that was asked and could not answer.
func (s *Store) RecordMiss(tier string) error {
	_, err := s.db.Exec(
		`INSERT INTO read_stats (tier, misses) VALUES (?, 1)
		 ON CONFLICT(tier) DO UPDATE SET misses = read_stats.misses + 1`,
		tier)
	if err != nil {
		return fmt.Errorf("count a %s miss: %w", tier, err)
	}
	return nil
}

// ReadStats reports how each tier of the read hierarchy has performed.
func (s *Store) ReadStats() ([]TierStat, error) {
	rows, err := s.db.Query(`SELECT tier, reads, nanos, misses FROM read_stats ORDER BY tier`)
	if err != nil {
		return nil, fmt.Errorf("read hierarchy statistics: %w", err)
	}
	defer rows.Close()

	var out []TierStat
	for rows.Next() {
		var stat TierStat
		var nanos int64
		if err := rows.Scan(&stat.Tier, &stat.Reads, &nanos, &stat.Misses); err != nil {
			return nil, fmt.Errorf("scan tier statistics: %w", err)
		}
		stat.Spent = time.Duration(nanos)
		out = append(out, stat)
	}
	return out, rows.Err()
}
