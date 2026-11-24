package tenantdb

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Column is one field of a user table.
type Column struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Nullable   bool   `json:"nullable"`
	PrimaryKey bool   `json:"primary_key"`
	Default    string `json:"default,omitempty"`
}

// Filter is one spreadsheet predicate. Values are always parameters.
type Filter struct {
	Column string `json:"column"`
	Op     string `json:"op"`
	Value  any    `json:"value,omitempty"`
}

// Browse is a page of a table.
type Browse struct {
	Schema  string
	Table   string
	Columns []Column
	Filters []Filter
	Sort    string
	Dir     string
	Limit   int
	Offset  int
}

var typeSQL = map[string]string{
	"text":        "text",
	"integer":     "integer",
	"bigint":      "bigint",
	"boolean":     "boolean",
	"numeric":     "numeric",
	"timestamptz": "timestamptz",
	"date":        "date",
	"uuid":        "uuid",
	"jsonb":       "jsonb",
}

// ColumnTypes is the picker list (id, plain label).
var ColumnTypes = []struct{ ID, Label string }{
	{"text", "Text"},
	{"integer", "Integer"},
	{"bigint", "Big integer"},
	{"boolean", "Yes / no"},
	{"numeric", "Number"},
	{"timestamptz", "Date and time"},
	{"date", "Date"},
	{"uuid", "UUID"},
	{"jsonb", "JSON"},
}

// NewColumn is a column on a table the user is creating.
type NewColumn struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Nullable   bool   `json:"nullable"`
	PrimaryKey bool   `json:"primary_key"`
	Default    string `json:"default,omitempty"`
}

func quoteRel(schema, table string) (string, error) {
	if !VisibleSchema(schema) || !SafeIdent(table) {
		return "", fmt.Errorf("that table isn't available")
	}
	return pgx.Identifier{schema, table}.Sanitize(), nil
}

func columnBy(cols []Column, name string) Column {
	for _, c := range cols {
		if c.Name == name {
			return c
		}
	}
	return Column{}
}

func coerceFilter(col Column, v any) any {
	s, ok := v.(string)
	if !ok {
		return v
	}
	switch col.Type {
	case "integer", "bigint", "smallint":
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return i
		}
	case "numeric":
		if !strings.ContainsAny(s, "eE") {
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				return f
			}
		}
	case "boolean":
		if s == "true" || s == "false" {
			return s == "true"
		}
	}
	return s
}

func known(cols []Column, name string) bool {
	for _, c := range cols {
		if c.Name == name {
			return true
		}
	}
	return false
}

func col(cols []Column, name string) (string, error) {
	if !known(cols, name) || !SafeIdent(name) {
		return "", fmt.Errorf("that column isn't on this table")
	}
	return pgx.Identifier{name}.Sanitize(), nil
}

// BuildSelect returns a parameterized page query and its count query.
func BuildSelect(b Browse) (page, count string, args []any, err error) {
	rel, err := quoteRel(b.Schema, b.Table)
	if err != nil {
		return "", "", nil, err
	}
	if len(b.Columns) == 0 {
		return "", "", nil, fmt.Errorf("that table has no columns")
	}
	names := make([]string, len(b.Columns))
	for i, c := range b.Columns {
		q, err := col(b.Columns, c.Name)
		if err != nil {
			return "", "", nil, err
		}
		names[i] = q
	}
	where, args, err := whereSQL(b.Columns, b.Filters)
	if err != nil {
		return "", "", nil, err
	}
	limit := b.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if b.Offset < 0 || b.Offset > 100_000 {
		return "", "", nil, fmt.Errorf("that page is out of range")
	}
	order := ""
	if b.Sort != "" {
		q, err := col(b.Columns, b.Sort)
		if err != nil {
			return "", "", nil, err
		}
		dir := "ASC"
		if strings.EqualFold(b.Dir, "desc") {
			dir = "DESC"
		} else if b.Dir != "" && !strings.EqualFold(b.Dir, "asc") {
			return "", "", nil, fmt.Errorf("sort direction must be asc or desc")
		}
		order = fmt.Sprintf(" ORDER BY %s %s NULLS LAST", q, dir)
	}
	n := len(args)
	pageArgs := append(append([]any{}, args...), limit, b.Offset)
	page = fmt.Sprintf("SELECT %s FROM %s%s%s LIMIT $%d OFFSET $%d",
		strings.Join(names, ", "), rel, where, order, n+1, n+2)
	count = fmt.Sprintf("SELECT count(*) FROM %s%s", rel, where)
	return page, count, pageArgs, nil
}

func whereSQL(cols []Column, filters []Filter) (string, []any, error) {
	if len(filters) > 8 {
		return "", nil, fmt.Errorf("use at most 8 filters")
	}
	var parts []string
	var args []any
	for _, f := range filters {
		q, err := col(cols, f.Column)
		if err != nil {
			return "", nil, err
		}
		switch f.Op {
		case "eq", "neq", "gt", "gte", "lt", "lte", "like", "ilike":
			op := map[string]string{"eq": "=", "neq": "<>", "gt": ">", "gte": ">=", "lt": "<", "lte": "<=", "like": "LIKE", "ilike": "ILIKE"}[f.Op]
			args = append(args, coerceFilter(columnBy(cols, f.Column), f.Value))
			parts = append(parts, fmt.Sprintf("%s %s $%d", q, op, len(args)))
		case "isnull":
			parts = append(parts, q+" IS NULL")
		case "notnull":
			parts = append(parts, q+" IS NOT NULL")
		default:
			return "", nil, fmt.Errorf("that filter isn't available")
		}
	}
	if len(parts) == 0 {
		return "", args, nil
	}
	return " WHERE " + strings.Join(parts, " AND "), args, nil
}

// BuildInsert returns INSERT ... RETURNING for a subset of columns.
func BuildInsert(schema, table string, cols []Column, values map[string]any) (string, []any, error) {
	rel, err := quoteRel(schema, table)
	if err != nil {
		return "", nil, err
	}
	if len(values) == 0 {
		return "", nil, fmt.Errorf("add a value for at least one column")
	}
	var names, marks []string
	var args []any
	for _, c := range cols {
		v, ok := values[c.Name]
		if !ok {
			continue
		}
		q, err := col(cols, c.Name)
		if err != nil {
			return "", nil, err
		}
		args = append(args, v)
		names = append(names, q)
		marks = append(marks, fmt.Sprintf("$%d", len(args)))
	}
	if len(names) == 0 {
		return "", nil, fmt.Errorf("none of those columns are on this table")
	}
	ret := returning(cols)
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING %s", rel, strings.Join(names, ", "), strings.Join(marks, ", "), ret), args, nil
}

// BuildUpdate changes one row identified by its primary key.
func BuildUpdate(schema, table string, cols []Column, pk, values map[string]any) (string, []any, error) {
	rel, err := quoteRel(schema, table)
	if err != nil {
		return "", nil, err
	}
	where, args, err := pkWhere(cols, pk)
	if err != nil {
		return "", nil, err
	}
	var sets []string
	for _, c := range cols {
		v, ok := values[c.Name]
		if !ok {
			continue
		}
		q, err := col(cols, c.Name)
		if err != nil {
			return "", nil, err
		}
		args = append(args, v)
		sets = append(sets, fmt.Sprintf("%s = $%d", q, len(args)))
	}
	if len(sets) == 0 {
		return "", nil, fmt.Errorf("change at least one column")
	}
	return fmt.Sprintf("UPDATE %s SET %s WHERE %s RETURNING %s", rel, strings.Join(sets, ", "), where, returning(cols)), args, nil
}

// BuildDelete removes one row by primary key.
func BuildDelete(schema, table string, cols []Column, pk map[string]any) (string, []any, error) {
	rel, err := quoteRel(schema, table)
	if err != nil {
		return "", nil, err
	}
	where, args, err := pkWhere(cols, pk)
	if err != nil {
		return "", nil, err
	}
	return fmt.Sprintf("DELETE FROM %s WHERE %s", rel, where), args, nil
}

func pkWhere(cols []Column, pk map[string]any) (string, []any, error) {
	var keys []Column
	for _, c := range cols {
		if c.PrimaryKey {
			keys = append(keys, c)
		}
	}
	if len(keys) == 0 {
		return "", nil, fmt.Errorf("this table needs a primary key before rows can be edited")
	}
	var parts []string
	var args []any
	for _, c := range keys {
		v, ok := pk[c.Name]
		if !ok || v == nil {
			return "", nil, fmt.Errorf("pick the row by its primary key")
		}
		q, err := col(cols, c.Name)
		if err != nil {
			return "", nil, err
		}
		args = append(args, v)
		parts = append(parts, fmt.Sprintf("%s = $%d", q, len(args)))
	}
	return strings.Join(parts, " AND "), args, nil
}

func returning(cols []Column) string {
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i], _ = col(cols, c.Name)
	}
	return strings.Join(names, ", ")
}

// BuildCreateTable returns CREATE TABLE for the type picker. Types and defaults
// come from a whitelist, never from raw SQL typed by the user.
func BuildCreateTable(name string, columns []NewColumn) (string, error) {
	if !SafeIdent(name) || vxRe.MatchString(name) {
		return "", fmt.Errorf("table names use letters, numbers and underscores, and can't start with vx_")
	}
	if len(columns) == 0 || len(columns) > 40 {
		return "", fmt.Errorf("add between 1 and 40 columns")
	}
	seen := map[string]bool{}
	var defs []string
	pk := ""
	for _, c := range columns {
		if !SafeIdent(c.Name) || seen[c.Name] {
			return "", fmt.Errorf("each column needs its own simple name")
		}
		seen[c.Name] = true
		sqlType, ok := typeSQL[c.Type]
		if !ok {
			return "", fmt.Errorf("pick a type from the list")
		}
		def, err := defaultExpr(c.Type, c.Default)
		if err != nil {
			return "", err
		}
		part := pgx.Identifier{c.Name}.Sanitize() + " " + sqlType
		if c.PrimaryKey {
			if pk != "" {
				return "", fmt.Errorf("pick one primary key column")
			}
			pk = c.Name
			part += " PRIMARY KEY"
		} else if !c.Nullable {
			part += " NOT NULL"
		}
		if def != "" {
			part += " DEFAULT " + def
		}
		defs = append(defs, part)
	}
	return fmt.Sprintf("CREATE TABLE %s (%s)", pgx.Identifier{"public", name}.Sanitize(), strings.Join(defs, ", ")), nil
}

// BuildAddColumn returns ALTER TABLE ... ADD COLUMN. Primary keys are chosen
// when the table is created; adding one later is a different, easier-to-misread change.
func BuildAddColumn(table string, c NewColumn) (string, error) {
	if !SafeIdent(table) {
		return "", fmt.Errorf("that table isn't available")
	}
	if c.PrimaryKey {
		return "", fmt.Errorf("add the primary key when you create the table")
	}
	stmt, err := BuildCreateTable("add_col_shape", []NewColumn{c})
	if err != nil {
		return "", err
	}
	open := strings.Index(stmt, "(")
	close := strings.LastIndex(stmt, ")")
	if open < 0 || close < 0 {
		return "", fmt.Errorf("could not add that column")
	}
	return fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", pgx.Identifier{"public", table}.Sanitize(), stmt[open+1:close]), nil
}

func defaultExpr(typ, def string) (string, error) {
	def = strings.TrimSpace(def)
	if def == "" {
		return "", nil
	}
	switch def {
	case "gen_random_uuid()":
		if typ != "uuid" {
			return "", fmt.Errorf("gen_random_uuid() is only for UUID columns")
		}
		return def, nil
	case "now()":
		if typ != "timestamptz" && typ != "date" {
			return "", fmt.Errorf("now() is only for date columns")
		}
		return def, nil
	case "true", "false":
		if typ != "boolean" {
			return "", fmt.Errorf("true and false are only for yes/no columns")
		}
		return def, nil
	default:
		if typ == "integer" || typ == "bigint" {
			if _, err := strconv.ParseInt(def, 10, 64); err != nil {
				return "", fmt.Errorf("that default isn't a whole number")
			}
			return def, nil
		}
		if typ == "numeric" {
			if _, err := strconv.ParseFloat(def, 64); err != nil || strings.ContainsAny(def, "eE") {
				return "", fmt.Errorf("that default isn't a number")
			}
			return def, nil
		}
		return "", fmt.Errorf("that default isn't available; leave it blank or pick one from the list")
	}
}
