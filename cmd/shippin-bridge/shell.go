package main

import (
	"errors"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

func shell(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: shippin-bridge ssh <workspace>")
	}
	c, err := loadCreds()
	if err != nil {
		return err
	}
	cl, _, err := connect(c, args[0])
	if err != nil {
		return err
	}
	defer cl.Close()
	s, err := cl.NewSession()
	if err != nil {
		return err
	}
	defer s.Close()
	s.Stdin, s.Stdout, s.Stderr = os.Stdin, os.Stdout, os.Stderr
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		old, err := term.MakeRaw(fd)
		if err == nil {
			defer term.Restore(fd, old)
		}
		w, h, _ := term.GetSize(fd)
		if err := s.RequestPty(envOr("TERM", "xterm-256color"), h, w, nil); err != nil {
			return err
		}
	}
	if err := s.Shell(); err != nil {
		return err
	}
	return s.Wait()
}

func execCmd(args []string) error {
	if len(args) < 3 || args[1] != "--" {
		return errors.New("usage: shippin-bridge exec <workspace> -- <command...>")
	}
	c, err := loadCreds()
	if err != nil {
		return err
	}
	cl, _, err := connect(c, args[0])
	if err != nil {
		return err
	}
	defer cl.Close()
	s, err := cl.NewSession()
	if err != nil {
		return err
	}
	defer s.Close()
	s.Stdin, s.Stdout, s.Stderr = os.Stdin, os.Stdout, os.Stderr
	return s.Run(strings.Join(args[2:], " "))
}

// proxy is for `ssh -o ProxyCommand="shippin-bridge proxy %h"`; that ssh still needs a Shippin
// certificate, so `shippin-bridge ssh` is the simpler path.
func proxy(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: shippin-bridge proxy <workspace>")
	}
	c, err := loadCreds()
	if err != nil {
		return err
	}
	w, err := resolve(c, args[0])
	if err != nil {
		return err
	}
	conn, err := dialRelay(c, w.ID)
	if err != nil {
		return err
	}
	defer conn.Close()
	go func() { _, _ = io.Copy(conn, os.Stdin) }()
	_, err = io.Copy(os.Stdout, conn)
	return err
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
