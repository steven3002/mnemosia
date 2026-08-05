//go:build ORT

package embed

import (
	"context"
	"fmt"

	"github.com/knights-analytics/hugot"
)

// backendName is the runtime compiled into this binary.
const backendName = "ort"

// newSession opens the ONNX Runtime backend, which needs cgo and a native
// runtime present on the machine. Scores are identical to the pure-Go backend
// for a full-precision model; the difference is speed alone.
func newSession(ctx context.Context) (*hugot.Session, error) {
	session, err := hugot.NewORTSession(ctx)
	if err != nil {
		return nil, fmt.Errorf("open ONNX Runtime embedding session: %w", err)
	}
	return session, nil
}
