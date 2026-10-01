//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// detached puts the command in a session of its own, out of reach of the
// hangup and interrupt aimed at the process that started it.
func detached(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }

// running says whether a process by this id exists.
func running(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	return err == nil || err == syscall.EPERM
}
