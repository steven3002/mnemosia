//go:build !windows

package embedtest

import (
	"os"
	"syscall"
)

func lockFile(file *os.File) error { return syscall.Flock(int(file.Fd()), syscall.LOCK_EX) }

func unlockFile(file *os.File) { syscall.Flock(int(file.Fd()), syscall.LOCK_UN) }
