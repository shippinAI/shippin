package tenantdb

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Class is the safety decision for one user statement.
type Class struct {
	Kind   string // read, write, ddl, destructive
	Reason string
}

// ConfirmError is returned when a destructive statement needs an explicit confirm.
type ConfirmError struct{ Kind string }

func (e *ConfirmError) Error() string {
	return "This statement removes or rewrites the shape of your data. Confirm to run it."
}

var (
	deniedFn = map[string]bool{
		"set_config": true, "pg_read_file": true, "pg_read_binary_file": true,
		"pg_ls_dir": true, "pg_stat_file": true, "lo_import": true, "lo_export": true,
		"dblink": true, "dblink_exec": true, "dblink_connect": true, "dblink_open": true,
		"dblink_send_query": true, "pg_terminate_backend": true, "pg_cancel_backend": true,
		"pg_reload_conf": true, "pg_rotate_logfile": true, "pg_read_server_files": true,
	}
	foreignDB = regexp.MustCompile(`(?i)tdb_[0-9a-f]{32}`)
	dbnameLit = regexp.MustCompile(`(?i)(dbname|database)\s*=\s*vaultix\b`)
)

// Classify accepts one statement and refuses session takeover, cross-database
// access, Vaultix schemas, and anything outside the spreadsheet/SQL surface.
func Classify(sql string, t Target) (Class, error) {
	if err := t.Check(); err != nil {
		return Class{}, err
	}
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return Class{}, fmt.Errorf("write a SQL statement first")
	}
	if len(sql) > 20_000 {
		return Class{}, fmt.Errorf("that statement is too long")
	}
	sc, err := scanSQL(sql, t)
	if err != nil {
		return Class{}, err
	}
	if sc.multi {
		return Class{}, fmt.Errorf("run one statement at a time")
	}
	if len(sc.keywords) == 0 {
		return Class{}, fmt.Errorf("write a SQL statement first")
	}
	for _, id := range sc.idents {
		if reason := badIdent(id, t); reason != "" {
			return Class{}, fmt.Errorf("%s", reason)
		}
		if deniedFn[strings.ToLower(id)] && !sc.quoted[id] {
			return Class{}, fmt.Errorf("that function isn't available in the console")
		}
	}
	// Denied functions are matched case-insensitively on unquoted words.
	for _, kw := range sc.keywords {
		if deniedFn[strings.ToLower(kw)] {
			return Class{}, fmt.Errorf("that function isn't available in the console")
		}
	}
	return classKeywords(sc.keywords)
}

func badIdent(id string, t Target) string {
	n := strings.ToLower(id)
	if vxRe.MatchString(n) {
		return "Vaultix schemas are not part of this database"
	}
	if tdbRe.MatchString(n) && !strings.EqualFold(n, t.Database) {
		return "that statement names another tenant's database"
	}
	if tnRe.MatchString(n) && !strings.EqualFold(n, t.Role) {
		return "that statement names another tenant's database role"
	}
	return ""
}

func classKeywords(kw []string) (Class, error) {
	head := strings.ToUpper(kw[0])
	has := func(w string) bool {
		for _, k := range kw {
			if strings.EqualFold(k, w) {
				return true
			}
		}
		return false
	}
	switch head {
	case "SELECT", "TABLE", "VALUES", "SHOW":
		if head == "SELECT" && has("INTO") {
			return Class{Kind: "write", Reason: "SELECT INTO writes a table"}, nil
		}
		return Class{Kind: "read"}, nil
	case "WITH":
		switch {
		case has("DELETE") || has("DROP") || has("TRUNCATE"):
			return Class{Kind: "destructive"}, nil
		case has("INSERT") || has("UPDATE") || has("MERGE"):
			return Class{Kind: "write"}, nil
		default:
			return Class{Kind: "read"}, nil
		}
	case "EXPLAIN":
		if has("ANALYZE") || has("ANALYSE") {
			if has("DELETE") || has("DROP") || has("TRUNCATE") {
				return Class{Kind: "destructive", Reason: "EXPLAIN ANALYZE runs the statement"}, nil
			}
			if has("INSERT") || has("UPDATE") || has("MERGE") {
				return Class{Kind: "write", Reason: "EXPLAIN ANALYZE runs the statement"}, nil
			}
		}
		return Class{Kind: "read"}, nil
	case "INSERT", "UPDATE", "MERGE":
		return Class{Kind: "write"}, nil
	case "DELETE", "TRUNCATE":
		return Class{Kind: "destructive"}, nil
	case "DROP":
		obj := ""
		if len(kw) > 1 {
			obj = strings.ToUpper(kw[1])
		}
		switch obj {
		case "TABLE", "INDEX", "VIEW", "SCHEMA":
			return Class{Kind: "destructive"}, nil
		default:
			return Class{}, fmt.Errorf("that DROP isn't available in the console")
		}
	case "CREATE":
		switch objectAfterCreate(kw) {
		case "TABLE", "INDEX", "VIEW", "SCHEMA":
			return Class{Kind: "ddl"}, nil
		default:
			return Class{}, fmt.Errorf("that CREATE isn't available in the console")
		}
	case "ALTER":
		if len(kw) < 2 || !strings.EqualFold(kw[1], "TABLE") {
			return Class{}, fmt.Errorf("only ALTER TABLE is available in the console")
		}
		if has("DROP") {
			return Class{Kind: "destructive"}, nil
		}
		return Class{Kind: "ddl"}, nil
	case "COMMENT":
		return Class{Kind: "ddl"}, nil
	default:
		return Class{}, fmt.Errorf("that statement isn't available in the console")
	}
}

func objectAfterCreate(kw []string) string {
	for i := 1; i < len(kw); i++ {
		switch strings.ToUpper(kw[i]) {
		case "OR", "REPLACE", "UNIQUE", "TEMP", "TEMPORARY", "UNLOGGED", "CONCURRENTLY", "IF", "NOT", "EXISTS":
			continue
		default:
			return strings.ToUpper(kw[i])
		}
	}
	return ""
}

// CheckRun applies read-only mode and the destructive confirmation gate.
func CheckRun(c Class, readOnly, confirm bool) error {
	if readOnly && c.Kind != "read" {
		return ErrReadOnly
	}
	if c.Kind == "destructive" && !confirm {
		return &ConfirmError{Kind: c.Kind}
	}
	return nil
}

type scanned struct {
	keywords []string
	idents   []string
	quoted   map[string]bool
	multi    bool
}

func scanSQL(sql string, t Target) (scanned, error) {
	sc := scanned{quoted: map[string]bool{}}
	s := sql
	i := 0
	afterSemi := false
	mark := func() error {
		if afterSemi {
			sc.multi = true
		}
		return nil
	}
	for i < len(s) {
		c := rune(s[i])
		if unicode.IsSpace(c) {
			i++
			continue
		}
		if strings.HasPrefix(s[i:], "--") {
			if n := strings.IndexByte(s[i:], '\n'); n < 0 {
				break
			} else {
				i += n + 1
			}
			continue
		}
		if strings.HasPrefix(s[i:], "/*") {
			n := strings.Index(s[i+2:], "*/")
			if n < 0 {
				return scanned{}, fmt.Errorf("that comment is missing its end")
			}
			i += 2 + n + 2
			continue
		}
		if c == '\'' {
			lit, next, err := readString(s, i)
			if err != nil {
				return scanned{}, err
			}
			if foreignInLiteral(lit, t) {
				return scanned{}, fmt.Errorf("that statement names another database")
			}
			_ = mark()
			i = next
			continue
		}
		if c == '"' {
			id, next, err := readQuoted(s, i)
			if err != nil {
				return scanned{}, err
			}
			sc.idents = append(sc.idents, id)
			sc.quoted[id] = true
			_ = mark()
			i = next
			continue
		}
		if c == '$' {
			body, next, isQuote, err := readDollar(s, i)
			if err != nil {
				return scanned{}, err
			}
			if isQuote {
				if foreignInLiteral(body, t) {
					return scanned{}, fmt.Errorf("that statement names another database")
				}
				_ = mark()
				i = next
				continue
			}
			i++
			continue
		}
		if c == ';' {
			afterSemi = true
			i++
			continue
		}
		if isIdentStart(byte(c)) {
			j := i + 1
			for j < len(s) && isIdentCont(s[j]) {
				j++
			}
			word := s[i:j]
			sc.idents = append(sc.idents, word)
			if !afterSemi {
				sc.keywords = append(sc.keywords, word)
			}
			_ = mark()
			i = j
			continue
		}
		_ = mark()
		i++
	}
	return sc, nil
}

func foreignInLiteral(lit string, t Target) bool {
	if dbnameLit.MatchString(lit) {
		return true
	}
	for _, m := range foreignDB.FindAllString(lit, -1) {
		if !strings.EqualFold(m, t.Database) {
			return true
		}
	}
	return false
}

func readString(s string, i int) (string, int, error) {
	var b strings.Builder
	for j := i + 1; j < len(s); j++ {
		if s[j] == '\'' {
			if j+1 < len(s) && s[j+1] == '\'' {
				b.WriteByte('\'')
				j++
				continue
			}
			return b.String(), j + 1, nil
		}
		b.WriteByte(s[j])
	}
	return "", 0, fmt.Errorf("that quote is missing its end")
}

func readQuoted(s string, i int) (string, int, error) {
	var b strings.Builder
	for j := i + 1; j < len(s); j++ {
		if s[j] == '"' {
			if j+1 < len(s) && s[j+1] == '"' {
				b.WriteByte('"')
				j++
				continue
			}
			return b.String(), j + 1, nil
		}
		b.WriteByte(s[j])
	}
	return "", 0, fmt.Errorf("that identifier is missing its end quote")
}

func readDollar(s string, i int) (body string, next int, isQuote bool, err error) {
	j := i + 1
	for j < len(s) && isIdentCont(s[j]) {
		j++
	}
	if j >= len(s) || s[j] != '$' {
		return "", 0, false, nil
	}
	tag := s[i : j+1]
	rest := s[j+1:]
	k := strings.Index(rest, tag)
	if k < 0 {
		return "", 0, true, fmt.Errorf("that quote is missing its end")
	}
	return rest[:k], j + 1 + k + len(tag), true, nil
}

func isIdentStart(b byte) bool {
	return b == '_' || (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

func isIdentCont(b byte) bool {
	return isIdentStart(b) || (b >= '0' && b <= '9')
}

// LimitSelect wraps a read statement so Postgres stops after max rows.
// SHOW and EXPLAIN cannot sit in a subquery; they rely on the session timeout.
func LimitSelect(sql string, kind string, max int) string {
	s := strings.TrimSpace(sql)
	s = strings.TrimSuffix(s, ";")
	s = strings.TrimSpace(s)
	if kind != "read" || max <= 0 {
		return s
	}
	head := ""
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\n' || s[i] == '\t' || s[i] == '(' {
			head = s[:i]
			break
		}
	}
	if head == "" {
		head = s
	}
	switch strings.ToUpper(head) {
	case "SHOW", "EXPLAIN":
		return s
	default:
		return fmt.Sprintf("SELECT * FROM (%s) AS shippin_result LIMIT %d", s, max)
	}
}
