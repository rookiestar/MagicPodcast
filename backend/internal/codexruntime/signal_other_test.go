//go:build !darwin && !linux

package codexruntime

import "syscall"

func signalIgnore(_ syscall.Signal) {}
