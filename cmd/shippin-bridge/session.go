package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/zalando/go-keyring"
)

// The bridge token lives in the OS keychain (macOS Keychain, Windows Credential Manager, Secret
// Service on Linux), never in a file. SHIPPIN_BRIDGE_TOKEN/SHIPPIN_URL override for headless use.
const keychainService = "shippin-bridge"

type creds struct {
	Server string `json:"server"`
	Token  string `json:"token"`
}

func loadCreds() (creds, error) {
	if t := os.Getenv("SHIPPIN_BRIDGE_TOKEN"); t != "" {
		return creds{Server: os.Getenv("SHIPPIN_URL"), Token: t}, nil
	}
	raw, err := keyring.Get(keychainService, "session")
	if err != nil {
		return creds{}, errors.New("not signed in: run shippin-bridge login")
	}
	var c creds
	if json.Unmarshal([]byte(raw), &c) != nil || c.Server == "" {
		return creds{}, errors.New("stored session unreadable: run shippin-bridge login")
	}
	return c, nil
}

func saveCreds(c creds) error {
	raw, _ := json.Marshal(c)
	if err := keyring.Set(keychainService, "session", string(raw)); err != nil {
		return fmt.Errorf("no OS keychain available (%v); set SHIPPIN_BRIDGE_TOKEN for this shell instead", err)
	}
	return nil
}

func logout() error { _ = keyring.Delete(keychainService, "session"); return nil }

var httpc = &http.Client{Timeout: 30 * time.Second}

// call sends a JSON request with the bridge token and decodes the reply into out.
func call(c creds, method, path string, in, out any) error {
	var body bytes.Buffer
	if in != nil {
		_ = json.NewEncoder(&body).Encode(in)
	}
	req, err := http.NewRequest(method, c.Server+path, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	res, err := httpc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		var p struct {
			Detail string `json:"detail"`
			Error  string `json:"error"`
		}
		_ = json.NewDecoder(res.Body).Decode(&p)
		return &apiError{res.StatusCode, p.Detail + p.Error}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(out)
}

type apiError struct {
	Status int
	Msg    string
}

func (e *apiError) Error() string { return fmt.Sprintf("%d %s", e.Status, e.Msg) }
