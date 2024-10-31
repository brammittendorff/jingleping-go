// +build linux darwin

package main

import (
	"syscall"
)

func exit() {
	syscall.Kill(syscall.Getpid(), syscall.SIGINT)
}
