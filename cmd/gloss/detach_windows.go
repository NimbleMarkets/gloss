//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

const detachedProcess = 0x00000008

// detached starts the command with no console and a group of its own.
func detached(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess}
}

// running says whether a process by this id exists: Windows cannot open a
// handle on one that has ended.
func running(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	p.Release()
	return true
}
