//go:build !linux

package main

import "fmt"

type syscallStatfs struct {
	Blocks uint64
	Bsize  int64
	Bavail uint64
}

func statfs(path string, stat *syscallStatfs) error {
	return fmt.Errorf("statfs is only available on Linux")
}

func filesystemDev(path string) (uint64, error) {
	return 0, fmt.Errorf("filesystemDev is only available on Linux")
}

func execCommand(name string, args ...string) (string, error) {
	return "", fmt.Errorf("command execution is only available on Linux")
}

func sudoExecCommand(password, name string, args ...string) (string, error) {
	return "", fmt.Errorf("sudo command execution is only available on Linux")
}
