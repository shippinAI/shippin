//go:build linux

package main

import "github.com/msteinert/pam"

func pamAuthenticate(username, password string) error {
	t, err := pam.StartFunc("login", username,
		func(style pam.Style, msg string) (string, error) {
			switch style {
			case pam.PromptEchoOff:
				return password, nil
			case pam.PromptEchoOn:
				return username, nil
			}
			return "", nil
		})
	if err != nil {
		return err
	}
	return t.Authenticate(0)
}
