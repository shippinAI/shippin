package app

import (
	"errors"
	"strings"
	"testing"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func files(m map[string]string) func(string) ([]byte, error) {
	return func(p string) ([]byte, error) {
		if v, ok := m[p]; ok {
			return []byte(v), nil
		}
		return nil, errors.New("no such file")
	}
}

// An installation names its owner with an email and a password, inline or in a file; naming none leaves
// the first visit to claim it.
func TestOwnerFromEnv(t *testing.T) {
	none := files(nil)
	if _, ok, err := ownerFromEnv(envOf(nil), none); ok || err != nil {
		t.Fatalf("no owner named: ok=%v err=%v", ok, err)
	}
	in, ok, err := ownerFromEnv(envOf(map[string]string{"SHIPPIN_OWNER_EMAIL": " ada@example.invalid ", "SHIPPIN_OWNER_PASSWORD": "a long test password"}), none)
	if err != nil || !ok || in.Email != "ada@example.invalid" || in.Name != "ada" || in.Password != "a long test password" || in.Organization != "" {
		t.Fatalf("inline: %+v %v %v", in, ok, err)
	}
	in, ok, err = ownerFromEnv(envOf(map[string]string{"SHIPPIN_OWNER_EMAIL": "ada@example.invalid", "SHIPPIN_OWNER_PASSWORD_FILE": "/run/pw",
		"SHIPPIN_OWNER_NAME": "Ada Lovelace", "SHIPPIN_OWNER_USERNAME": "ada", "SHIPPIN_ORGANIZATION": "Analytical"}), files(map[string]string{"/run/pw": "from a file\r\n"}))
	if err != nil || !ok || in.Password != "from a file" || in.Name != "Ada Lovelace" || in.Username != "ada" || in.Organization != "Analytical" {
		t.Fatalf("file: %+v %v %v", in, ok, err)
	}
	for name, env := range map[string]map[string]string{
		"no password":   {"SHIPPIN_OWNER_EMAIL": "ada@example.invalid"},
		"both":          {"SHIPPIN_OWNER_EMAIL": "ada@example.invalid", "SHIPPIN_OWNER_PASSWORD": "x", "SHIPPIN_OWNER_PASSWORD_FILE": "/run/pw"},
		"missing file":  {"SHIPPIN_OWNER_EMAIL": "ada@example.invalid", "SHIPPIN_OWNER_PASSWORD_FILE": "/nope"},
		"empty in file": {"SHIPPIN_OWNER_EMAIL": "ada@example.invalid", "SHIPPIN_OWNER_PASSWORD_FILE": "/empty"},
	} {
		if _, ok, err := ownerFromEnv(envOf(env), files(map[string]string{"/run/pw": "p", "/empty": "\n"})); ok || err == nil || !strings.Contains(err.Error(), "SHIPPIN_OWNER") {
			t.Errorf("%s: ok=%v err=%v", name, ok, err)
		}
	}
}
