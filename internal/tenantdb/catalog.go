package tenantdb

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Table is one user-visible relation.
type Table struct {
	Schema   string `json:"schema"`
	Name     string `json:"name"`
	RowCount *int64 `json:"row_count"`
	Estimate int64  `json:"estimate"`
	Platform bool   `json:"platform"`
	Comment  string `json:"comment,omitempty"`
}

// listTablesSQL hides catalogs and Vaultix schemas even if a tenant role could see them.
const listTablesSQL = `
SELECT n.nspname, c.relname, COALESCE(c.reltuples, 0)::bigint,
       COALESCE(obj_description(c.oid, 'pg_class'), '')
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind = 'r'
  AND n.nspname <> 'information_schema'
  AND n.nspname <> 'pg_catalog'
  AND n.nspname <> 'vaultix'
  AND n.nspname NOT LIKE 'pg\_%'
  AND n.nspname NOT LIKE 'vx\_%'
ORDER BY n.nspname, c.relname`

const columnsSQL = `
SELECT c.column_name, c.udt_name, c.is_nullable = 'YES', COALESCE(c.column_default, ''),
       EXISTS (
         SELECT 1
         FROM information_schema.table_constraints tc
         JOIN information_schema.key_column_usage k
           ON k.constraint_name = tc.constraint_name
          AND k.table_schema = tc.table_schema
          AND k.table_name = tc.table_name
         WHERE tc.constraint_type = 'PRIMARY KEY'
           AND tc.table_schema = c.table_schema
           AND tc.table_name = c.table_name
           AND k.column_name = c.column_name
       )
FROM information_schema.columns c
WHERE c.table_schema = $1 AND c.table_name = $2
ORDER BY c.ordinal_position`

// Gate runs every user-facing database operation through Open.
type Gate struct {
	Dial   Dialer
	Limits Guard
}

// NewGate builds a gate. A nil dialer means queries are not configured.
func NewGate(d Dialer, g Guard) *Gate {
	return &Gate{Dial: d, Limits: g.withDefaults()}
}

func (g *Gate) limits(readOnly bool) Guard {
	out := g.Limits.withDefaults()
	out.ReadOnly = readOnly
	return out
}

// Tables lists relations in visible schemas and counts rows.
func (g *Gate) Tables(ctx context.Context, tenantID string) ([]Table, error) {
	c, _, err := Open(ctx, g.Dial, tenantID, g.limits(true))
	if err != nil {
		return nil, err
	}
	defer c.Close()
	got, err := c.Query(ctx, listTablesSQL)
	if err != nil {
		return nil, err
	}
	var out []Table
	for _, row := range got.Data {
		if len(row) < 4 {
			continue
		}
		schema, _ := row[0].(string)
		name, _ := row[1].(string)
		if !VisibleSchema(schema) || !SafeIdent(name) {
			continue
		}
		est, _ := asInt(row[2])
		comment, _ := row[3].(string)
		t := Table{Schema: schema, Name: name, Estimate: est, Platform: PlatformTable(name) && schema == "public", Comment: comment}
		countSQL := fmt.Sprintf("SELECT count(*) FROM %s", pgx.Identifier{schema, name}.Sanitize())
		counted, err := c.Query(ctx, countSQL)
		if err == nil && len(counted.Data) == 1 {
			if n, ok := asInt(counted.Data[0][0]); ok {
				t.RowCount = &n
			}
		}
		out = append(out, t)
	}
	if out == nil {
		out = []Table{}
	}
	return out, nil
}

// LoadColumns reads the catalog for one visible table.
func (g *Gate) LoadColumns(ctx context.Context, c Conn, schema, table string) ([]Column, error) {
	if !VisibleSchema(schema) || !SafeIdent(table) {
		return nil, fmt.Errorf("that table isn't available")
	}
	got, err := c.Query(ctx, columnsSQL, schema, table)
	if err != nil {
		return nil, err
	}
	var cols []Column
	for _, row := range got.Data {
		if len(row) < 5 {
			continue
		}
		name, _ := row[0].(string)
		typ, _ := row[1].(string)
		if !SafeIdent(name) {
			continue
		}
		nullable, _ := row[2].(bool)
		def, _ := row[3].(string)
		pk, _ := row[4].(bool)
		cols = append(cols, Column{Name: name, Type: friendlyType(typ), Nullable: nullable, PrimaryKey: pk, Default: def})
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("that table doesn't exist yet")
	}
	return cols, nil
}

func friendlyType(udt string) string {
	switch udt {
	case "int4":
		return "integer"
	case "int8":
		return "bigint"
	case "int2":
		return "smallint"
	case "bool":
		return "boolean"
	case "timestamptz":
		return "timestamptz"
	case "varchar", "bpchar":
		return "text"
	default:
		return udt
	}
}

func asInt(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int32:
		return int64(n), true
	case int:
		return int64(n), true
	case float64:
		return int64(n), true
	case string:
		i, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		return i, err == nil
	default:
		return 0, false
	}
}
