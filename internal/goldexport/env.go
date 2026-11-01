package goldexport

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// KEY=value lines, comments allowed, no shell evaluation.
// ReadEnv is the same KEY=value parser gold-passkey-verify uses.
func ReadEnv(path string, required ...string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	env := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if i := strings.Index(line, "#"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			env[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	for _, k := range required {
		if env[k] == "" {
			return nil, fmt.Errorf("%s: %s is required", path, k)
		}
	}
	return env, sc.Err()
}
