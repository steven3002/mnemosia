package local

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/steven3002/mnemosia/record"
)

// ErrNotFound reports that the device has no copy of a record.
var ErrNotFound = errors.New("not held locally")

// PutBody stores a record body on the device.
//
// This is the step that makes a write durable for the user. Reaching Sia is a
// separate, later event, and the difference between the two is a window the
// interface must show rather than hide.
func (s *Store) PutBody(id record.ID, kind record.Kind, body []byte) error {
	_, err := s.db.Exec(
		`INSERT INTO bodies (record_id, kind, body, created_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(record_id) DO UPDATE SET body = excluded.body`,
		id.String(), string(kind), body, record.Now().String())
	if err != nil {
		return fmt.Errorf("store body %s: %w", id, err)
	}
	return nil
}

// GetBody reads a record body from the device.
func (s *Store) GetBody(id record.ID) ([]byte, error) {
	var body []byte
	err := s.db.QueryRow(`SELECT body FROM bodies WHERE record_id = ?`, id.String()).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("body %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("read body %s: %w", id, err)
	}
	return body, nil
}

// CountBodies reports how many records the device holds.
func (s *Store) CountBodies() (int, error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM bodies`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count bodies: %w", err)
	}
	return n, nil
}
