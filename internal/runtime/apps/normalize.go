package apps

import "regexp"

var versionRe = regexp.MustCompile(`^[0-9A-Za-z.+~:-]{1,64}$`)
var userRe = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

// Normalize drops anything the guest status script was not supposed to report and forces
// signed-in to unknown. Guest output is untrusted.
func Normalize(st *Status) {
	if st == nil {
		return
	}
	st.SignedIn = "unknown"
	next := map[string]string{}
	for _, pkg := range []string{"claude-desktop", "claude-code", "chatgpt"} {
		if v := st.Packages[pkg]; versionRe.MatchString(v) {
			next[pkg] = v
		}
	}
	st.Packages = next
	users := make([]UserStatus, 0, len(st.Users))
	for _, u := range st.Users {
		if userRe.MatchString(u.User) && u.UID >= 10000 && u.UID <= 59999 {
			users = append(users, u)
		}
	}
	st.Users = users
}
