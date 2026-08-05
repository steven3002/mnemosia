// Command mnemosia-mcp serves a vault over the Model Context Protocol.
package main

import (
	"fmt"
	"os"

	"github.com/steven3002/mnemosia/mcp"
)

func main() {
	fmt.Fprintf(os.Stderr, "mnemosia-mcp: %v\n", mcp.ErrNotServing)
	os.Exit(1)
}
