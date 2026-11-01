package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type destSecrets struct {
	AccessKey string `json:"accessKey,omitempty"`
	SecretKey string `json:"secretKey,omitempty"`
}

var (
	destSecretsMu   sync.RWMutex
	destSecretsByID = map[string]destSecrets{}
)

func destSecretsPath() string {
	return filepath.Join(filepath.Dir(backupPath), "backups-secrets.json")
}

func loadDestSecrets() {
	data, err := os.ReadFile(destSecretsPath())
	if err != nil {
		return
	}
	var parsed map[string]destSecrets
	if err := json.Unmarshal(data, &parsed); err != nil {
		log.Printf("Failed to parse backup secrets file")
		return
	}
	destSecretsMu.Lock()
	destSecretsByID = parsed
	if destSecretsByID == nil {
		destSecretsByID = map[string]destSecrets{}
	}
	destSecretsMu.Unlock()
}

func saveDestSecrets() error {
	destSecretsMu.RLock()
	data, err := json.MarshalIndent(destSecretsByID, "", "  ")
	destSecretsMu.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(destSecretsPath(), data, 0600)
}

func mergeDestSecrets(id, access, secret string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	destSecretsMu.Lock()
	cur := destSecretsByID[id]
	if strings.TrimSpace(access) != "" {
		cur.AccessKey = strings.TrimSpace(access)
	}
	if strings.TrimSpace(secret) != "" {
		cur.SecretKey = strings.TrimSpace(secret)
	}
	if destSecretsByID == nil {
		destSecretsByID = map[string]destSecrets{}
	}
	destSecretsByID[id] = cur
	destSecretsMu.Unlock()
	return saveDestSecrets()
}

func destAccessKey(id string) string {
	destSecretsMu.RLock()
	v := destSecretsByID[id].AccessKey
	destSecretsMu.RUnlock()
	if v != "" {
		return v
	}
	return s3AccessKey()
}

func destSecretKey(id string) string {
	destSecretsMu.RLock()
	v := destSecretsByID[id].SecretKey
	destSecretsMu.RUnlock()
	if v != "" {
		return v
	}
	return s3SecretKey()
}

func destHasUIKeys(id string) bool {
	destSecretsMu.RLock()
	defer destSecretsMu.RUnlock()
	s := destSecretsByID[id]
	return s.AccessKey != "" && s.SecretKey != ""
}

func resetDestSecrets() {
	destSecretsMu.Lock()
	destSecretsByID = map[string]destSecrets{}
	destSecretsMu.Unlock()
}
