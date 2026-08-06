//go:build windows

package embedtest

import "os"

// There is no advisory file lock here, so two test binaries can hold the model
// at once on Windows. That is a memory ceiling rather than a correctness
// problem: the suite still passes, on a machine with room for two models.
func lockFile(*os.File) error { return nil }

func unlockFile(*os.File) {}
