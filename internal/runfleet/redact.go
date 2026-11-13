package runfleet

import (
	"regexp"
	"strings"
)

const redacted = "«redacted»"

var redactPatterns = []*regexp.Regexp{
	regexp.MustCompile(`ghp_[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`gho_[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`ghu_[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`ghs_[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`ghr_[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`),
	regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._\-]{8,}`),
	regexp.MustCompile(`-----BEGIN [A-Z ]+PRIVATE KEY-----[\s\S]*?-----END [A-Z ]+PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)\b(aws_secret_access_key|password|passwd|secret|api[_-]?key|token)\b(\s*[:=]\s*)["']?[^\s"']+`),
}

// Redact replaces credential-shaped text. The count is how many replacements
// were made. Ordinary words, including the word "token" with no value, stay.
func Redact(in string) (string, int) {
	n := 0
	out := in
	for i, re := range redactPatterns {
		out = re.ReplaceAllStringFunc(out, func(match string) string {
			n++
			if i == len(redactPatterns)-1 {
				parts := re.FindStringSubmatch(match)
				if len(parts) == 3 {
					return parts[1] + parts[2] + redacted
				}
			}
			return redacted
		})
	}
	return out, n
}

// RedactLog is Redact plus a trailing newline so stored logs stay text files.
func RedactLog(in string) (string, int) {
	text, n := Redact(in)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text, n
}
