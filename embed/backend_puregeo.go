//go:build !ORT

package embed

import (
	"context"
	"fmt"

	"github.com/knights-analytics/hugot"
)

// backendName is the runtime compiled into this binary.
const backendName = "go"

// newSession opens the pure-Go backend.
//
// This is the default because a dependency-free install is a product property:
// the whole library has to build with cgo disabled and work after a plain go
// get, with no native runtime to locate first. The faster backend is a build
// tag away for anyone who wants to pay that cost.
func newSession(ctx context.Context) (*hugot.Session, error) {
	session, err := hugot.NewGoSession(ctx)
	if err != nil {
		return nil, fmt.Errorf("open pure-Go embedding session: %w", err)
	}
	return session, nil
}
