package local

import (
	"fmt"
	"time"

	"github.com/steven3002/mnemosia/record"
)

// A TrackedSlab is a slab this vault has pinned.
type TrackedSlab struct {
	SlabID   string
	PinnedAt time.Time
	Records  int
	Bytes    int64
}

// TrackSlab records that a flush pinned a slab.
//
// Every flush strands a slab that can never be extended, so this list is the
// difference between storage that can be reclaimed later and storage that leaks
// a full slab's worth of quota per flush with nothing to show which ones.
func (s *Store) TrackSlab(slabID string, records int, bytes int64) error {
	_, err := s.db.Exec(
		`INSERT INTO slabs (slab_id, pinned_at, records, bytes) VALUES (?, ?, ?, ?)
		 ON CONFLICT(slab_id) DO UPDATE SET records = slabs.records + excluded.records, bytes = slabs.bytes + excluded.bytes`,
		slabID, record.Now().String(), records, bytes)
	if err != nil {
		return fmt.Errorf("track slab %s: %w", slabID, err)
	}
	return nil
}

// TrackedSlabs lists every slab this vault has pinned, oldest first.
func (s *Store) TrackedSlabs() ([]TrackedSlab, error) {
	rows, err := s.db.Query(`SELECT slab_id, pinned_at, records, bytes FROM slabs ORDER BY pinned_at`)
	if err != nil {
		return nil, fmt.Errorf("read tracked slabs: %w", err)
	}
	defer rows.Close()

	var out []TrackedSlab
	for rows.Next() {
		var slab TrackedSlab
		var pinnedAt string
		if err := rows.Scan(&slab.SlabID, &pinnedAt, &slab.Records, &slab.Bytes); err != nil {
			return nil, fmt.Errorf("scan tracked slab: %w", err)
		}
		parsed, err := time.Parse(time.RFC3339, pinnedAt)
		if err != nil {
			return nil, fmt.Errorf("slab %s has an unreadable timestamp %q: %w", slab.SlabID, pinnedAt, err)
		}
		slab.PinnedAt = parsed
		out = append(out, slab)
	}
	return out, rows.Err()
}

// ForgetSlab drops a slab from the tracking list once it has been released.
func (s *Store) ForgetSlab(slabID string) error {
	if _, err := s.db.Exec(`DELETE FROM slabs WHERE slab_id = ?`, slabID); err != nil {
		return fmt.Errorf("forget slab %s: %w", slabID, err)
	}
	return nil
}
