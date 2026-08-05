package local

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/steven3002/mnemosia/record"
)

// A StoredVector is one record's embedding as held on the device.
type StoredVector struct {
	ID     record.ID
	Model  string
	Dim    int
	Values []float32
}

// PutVector stores a record's embedding.
//
// The model name is stored beside the vector rather than assumed, so switching
// models is a detectable re-index instead of a silent mixing of two embedding
// spaces that would degrade ranking without ever erroring.
func (s *Store) PutVector(id record.ID, model string, values []float32) error {
	_, err := s.db.Exec(
		`INSERT INTO vectors (record_id, model, dim, vector) VALUES (?, ?, ?, ?)
		 ON CONFLICT(record_id) DO UPDATE SET model = excluded.model, dim = excluded.dim, vector = excluded.vector`,
		id.String(), model, len(values), encodeVector(values))
	if err != nil {
		return fmt.Errorf("store vector %s: %w", id, err)
	}
	return nil
}

// Vectors reads every stored embedding, for hydrating the in-memory index.
func (s *Store) Vectors() ([]StoredVector, error) {
	rows, err := s.db.Query(`SELECT record_id, model, dim, vector FROM vectors`)
	if err != nil {
		return nil, fmt.Errorf("read vectors: %w", err)
	}
	defer rows.Close()

	var out []StoredVector
	for rows.Next() {
		var idHex, model string
		var dim int
		var blob []byte
		if err := rows.Scan(&idHex, &model, &dim, &blob); err != nil {
			return nil, fmt.Errorf("scan vector: %w", err)
		}
		id, err := record.ParseID(idHex)
		if err != nil {
			return nil, err
		}
		values, err := decodeVector(blob, dim)
		if err != nil {
			return nil, fmt.Errorf("vector %s: %w", idHex, err)
		}
		out = append(out, StoredVector{ID: id, Model: model, Dim: dim, Values: values})
	}
	return out, rows.Err()
}

func encodeVector(values []float32) []byte {
	out := make([]byte, 4*len(values))
	for i, v := range values {
		binary.LittleEndian.PutUint32(out[4*i:], math.Float32bits(v))
	}
	return out
}

func decodeVector(blob []byte, dim int) ([]float32, error) {
	if len(blob) != 4*dim {
		return nil, fmt.Errorf("%d bytes for %d dimensions", len(blob), dim)
	}
	values := make([]float32, dim)
	for i := range values {
		values[i] = math.Float32frombits(binary.LittleEndian.Uint32(blob[4*i:]))
	}
	return values, nil
}
