package api

import (
	"net/url"
	"strings"
)

// localPath returns rd if it is a same-origin path, else "/". Browsers treat "/\host",
// "/<TAB>/host" and "//host" as another origin, so backslashes (raw or %5C), control characters
// and anything with a scheme or host are refused.
func localPath(rd string) string {
	if !strings.HasPrefix(rd, "/") || strings.HasPrefix(rd, "//") {
		return "/"
	}
	for _, c := range rd {
		if c < 0x20 || c == 0x7f || c == '\\' {
			return "/"
		}
	}
	u, err := url.Parse(rd)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil ||
		strings.Contains(u.Path, "\\") || strings.HasPrefix(u.Path, "//") {
		return "/"
	}
	return rd
}
