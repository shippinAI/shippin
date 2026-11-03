package api

import (
	"regexp"
	"strconv"
)

// uuidRe matches a canonical lowercase or uppercase UUID (any version).
var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func itoa(n int) string { return strconv.Itoa(n) }
