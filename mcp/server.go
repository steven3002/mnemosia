// Package mcp exposes the vault over the Model Context Protocol.
package mcp

import (
	"errors"

	"github.com/steven3002/mnemosia/vault"
)

// ErrNotServing reports that the protocol surface is not implemented in this
// build. It is stated rather than approximated: a server that accepts a
// connection and then answers nothing useful is worse than one that declines.
var ErrNotServing = errors.New("the MCP surface is not implemented in this build")

// A Server exposes one vault to a protocol client.
//
// Each client launches its own process against the same vault, which is why the
// device store is multi-process safe from the outset rather than as a later
// hardening pass.
type Server struct {
	vault *vault.Vault
}

// New builds a server over a vault.
func New(v *vault.Vault) *Server { return &Server{vault: v} }
