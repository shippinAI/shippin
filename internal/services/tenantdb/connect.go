package tenantdb

import "fmt"

// ConnInfo is what the connection panel may show. It never includes a password.
type ConnInfo struct {
	Database           string            `json:"database"`
	Role               string            `json:"role"`
	Host               string            `json:"host"`
	Port               int               `json:"port"`
	SSLMode            string            `json:"sslmode"`
	URI                string            `json:"uri"`
	LoginReady         bool              `json:"login_ready"`
	StatementTimeoutMS int               `json:"statement_timeout_ms"`
	MaxRows            int               `json:"max_rows"`
	PasswordNote       string            `json:"password_note"`
	Hints              []string          `json:"hints"`
	Snippets           map[string]string `json:"snippets"`
}

// Describe builds the connection panel for a tenant. password is never accepted.
func Describe(tenantID, host string, port int, sslmode string, loginReady bool, timeoutMS, maxRows int) (ConnInfo, error) {
	t, err := TargetOf(tenantID)
	if err != nil {
		return ConnInfo{}, err
	}
	if port == 0 {
		port = 5432
	}
	if sslmode == "" {
		sslmode = "require"
	}
	if host == "" {
		host = "YOUR_POSTGRES_HOST"
	}
	if timeoutMS <= 0 {
		timeoutMS = 8000
	}
	if maxRows <= 0 {
		maxRows = 200
	}
	uri := fmt.Sprintf("postgresql://%s:[password]@%s:%d/%s?sslmode=%s", t.Role, host, port, t.Database, sslmode)
	info := ConnInfo{
		Database: t.Database, Role: t.Role, Host: host, Port: port, SSLMode: sslmode,
		URI: uri, LoginReady: loginReady, StatementTimeoutMS: timeoutMS, MaxRows: maxRows,
		PasswordNote: "The password is kept by the platform (sops via hostd) and is not shown here. Queries you run in this console sign in as " + t.Role + " on " + t.Database + " only.",
		Hints: []string{
			"This database belongs to your organization. Other organizations have their own.",
			"Vaultix secret storage is a different database and does not appear here.",
			fmt.Sprintf("Each statement stops after %d seconds and grids show at most %d rows.", timeoutMS/1000, maxRows),
		},
		Snippets: snippets(uri),
	}
	return info, nil
}

func snippets(uri string) map[string]string {
	return map[string]string{
		"javascript": fmt.Sprintf(`import postgres from "postgres";

const sql = postgres(%q);
const rows = await sql`+"`select id, title from todos`"+`;
console.log(rows);`, uri),
		"python": fmt.Sprintf(`import psycopg

with psycopg.connect(%q) as conn:
    rows = conn.execute("select id, title from todos").fetchall()
    print(rows)`, uri),
		"go": fmt.Sprintf(`package main

import (
    "database/sql"
    "fmt"

    _ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
    db, err := sql.Open("pgx", %q)
    if err != nil {
        panic(err)
    }
    defer db.Close()
    rows, err := db.Query("select id, title from todos")
    if err != nil {
        panic(err)
    }
    defer rows.Close()
    fmt.Println(rows)
}`, uri),
	}
}
