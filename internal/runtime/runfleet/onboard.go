package runfleet

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	accountName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}$`)
	repoName    = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
)

func validAccount(s string) bool { return accountName.MatchString(s) }

func validRepo(s string) bool {
	if !repoName.MatchString(s) || strings.Contains(s, "..") {
		return false
	}
	owner, name, _ := strings.Cut(s, "/")
	return owner != "" && name != "" && owner != "." && name != "."
}

// RegistrationLevel chooses organization runners only for organizations.
// A personal account always falls back to one registration per repository.
func RegistrationLevel(accountKind, requested string) (level, note string, err error) {
	switch accountKind {
	case "user", "organization":
	default:
		return "", "", fmt.Errorf("account_kind must be user or organization")
	}
	if requested == "" {
		if accountKind == "organization" {
			requested = "organization"
		} else {
			requested = "repository"
		}
	}
	switch requested {
	case "organization", "repository":
	default:
		return "", "", fmt.Errorf("registration must be repository or organization")
	}
	if accountKind == "user" && requested == "organization" {
		return "repository", "personal accounts have no organization runner level; each repository is registered on its own", nil
	}
	if requested == "organization" {
		return "organization", "organization runners cover the approved repositories", nil
	}
	return "repository", "each repository gets its own runner", nil
}

// Approval decides whether a repository may receive a runner without an operator.
// Public repositories with a detected license are the automatic path.
func Approval(fact RepoFact, autoOSS bool) (status, reason string) {
	if fact.Private {
		return "pending", "private repositories wait for an operator approval"
	}
	if fact.License == "" {
		return "pending", "public repository has no detected license"
	}
	if !autoOSS {
		return "pending", "automatic approval for public repositories is turned off"
	}
	return "auto", "public repository with a detected license"
}
