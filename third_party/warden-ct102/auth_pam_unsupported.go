//go:build !linux

package main

import "errors"

func pamAuthenticate(username, password string) error {
	return errors.New("PAM authentication is only available on Linux")
}
