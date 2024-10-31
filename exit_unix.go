// +build linux darwin

package main

import (
	"syscall"
	"os"
)

func exit() {
	syscall.Kill(syscall.Getpid(), syscall.SIGINT)
}
