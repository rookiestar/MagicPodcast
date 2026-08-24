//go:build darwin || linux

package codexruntime

import (
	"os/signal"
	"syscall"
)

func signalIgnore(value syscall.Signal) {
	signal.Ignore(value)
}
