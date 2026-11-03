package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const defaultCmdTimeout = 10 * time.Minute

type syscallStatfs struct {
	Blocks uint64
	Bsize  int64
	Bavail uint64
}

func statfs(path string, stat *syscallStatfs) error {
	var s syscall.Statfs_t
	if err := syscall.Statfs(path, &s); err != nil {
		return err
	}
	stat.Blocks = s.Blocks
	stat.Bsize = s.Bsize
	stat.Bavail = s.Bavail
	return nil
}

func filesystemDev(path string) (uint64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("stat_t unavailable for %s", path)
	}
	return uint64(st.Dev), nil
}

func execCommand(name string, args ...string) (string, error) {
	log.Printf("[cmd] exec: %s %s", name, strings.Join(args, " "))
	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), defaultCmdTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	elapsed := time.Since(start)

	if err != nil {
		log.Printf("[cmd] exec failed (%.1fs): %s %s: %v", elapsed.Seconds(), name, strings.Join(args, " "), err)
	} else {
		log.Printf("[cmd] exec done (%.1fs): %s %s", elapsed.Seconds(), name, strings.Join(args, " "))
	}

	if ctx.Err() == context.DeadlineExceeded {
		return string(out), fmt.Errorf("command timed out after %s", defaultCmdTimeout)
	}
	return string(out), err
}

// sudoExecCommand runs a command via sudo, piping the user's password for authentication.
// If the process is already running as root, sudo is skipped.
func sudoExecCommand(password, name string, args ...string) (string, error) {
	// If already root, no need for sudo
	if os.Getuid() == 0 {
		return execCommand(name, args...)
	}

	log.Printf("[cmd] sudo exec: %s %s", name, strings.Join(args, " "))
	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), defaultCmdTimeout)
	defer cancel()

	sudoArgs := append([]string{"-S", "--", name}, args...)
	cmd := exec.CommandContext(ctx, "sudo", sudoArgs...)
	cmd.Stdin = bytes.NewBufferString(password + "\n")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	elapsed := time.Since(start)

	if ctx.Err() == context.DeadlineExceeded {
		log.Printf("[cmd] sudo exec TIMEOUT (%.1fs): %s %s", elapsed.Seconds(), name, strings.Join(args, " "))
		return stdout.String(), fmt.Errorf("command timed out after %s", defaultCmdTimeout)
	}

	if err != nil {
		log.Printf("[cmd] sudo exec failed (%.1fs): %s %s: %v", elapsed.Seconds(), name, strings.Join(args, " "), err)
		// Return stderr (excluding the password prompt) for error context
		errMsg := stderr.String()
		// Filter out sudo's password prompt line
		lines := bytes.Split([]byte(errMsg), []byte("\n"))
		var filtered []string
		for _, line := range lines {
			s := string(line)
			if s != "" && !bytes.Contains(line, []byte("[sudo] password")) {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) > 0 {
			return strings.Join(filtered, "\n"), err
		}
		return stdout.String(), err
	}

	log.Printf("[cmd] sudo exec done (%.1fs): %s %s", elapsed.Seconds(), name, strings.Join(args, " "))
	return stdout.String(), nil
}
