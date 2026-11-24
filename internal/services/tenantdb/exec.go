package tenantdb

import (
	"context"
	"strings"
)

// Result is a statement outcome safe to show in the grid.
type Result struct {
	Columns   []string `json:"columns"`
	Rows      [][]any  `json:"rows"`
	RowCount  int      `json:"row_count"`
	Truncated bool     `json:"truncated"`
	Kind      string   `json:"kind"`
}

// Query runs one user statement as the tenant role.
func (g *Gate) Query(ctx context.Context, tenantID, sql string, readOnly, confirm bool) (Result, error) {
	t, err := TargetOf(tenantID)
	if err != nil {
		return Result{}, err
	}
	class, err := Classify(sql, t)
	if err != nil {
		return Result{}, err
	}
	if err := CheckRun(class, readOnly, confirm); err != nil {
		return Result{}, err
	}
	c, _, err := Open(ctx, g.Dial, tenantID, g.limits(readOnly))
	if err != nil {
		return Result{}, err
	}
	defer c.Close()
	return runClassified(ctx, c, class, sql, g.limits(readOnly).MaxRows)
}

func runClassified(ctx context.Context, c Conn, class Class, sql string, max int) (Result, error) {
	if class.Kind == "read" || strings.Contains(strings.ToUpper(sql), "RETURNING") {
		wrapped := sql
		if class.Kind == "read" {
			wrapped = LimitSelect(sql, class.Kind, max)
		}
		got, err := c.Query(ctx, wrapped)
		if err != nil {
			return Result{}, err
		}
		trunc := len(got.Data) > max
		data := got.Data
		if trunc {
			data = data[:max]
		}
		if data == nil {
			data = [][]any{}
		}
		return Result{Columns: got.Cols, Rows: data, RowCount: len(data), Truncated: trunc, Kind: class.Kind}, nil
	}
	n, err := c.Exec(ctx, LimitSelect(sql, class.Kind, max))
	if err != nil {
		return Result{}, err
	}
	return Result{Columns: []string{}, Rows: [][]any{}, RowCount: int(n), Kind: class.Kind}, nil
}

// Page is one spreadsheet page.
type Page struct {
	Schema   string           `json:"schema"`
	Table    string           `json:"table"`
	Columns  []Column         `json:"columns"`
	Rows     []map[string]any `json:"rows"`
	Total    int64            `json:"total"`
	Limit    int              `json:"limit"`
	Offset   int              `json:"offset"`
	Platform bool             `json:"platform"`
}

// Browse reads one page as the tenant role.
func (g *Gate) Browse(ctx context.Context, tenantID string, b Browse) (Page, error) {
	c, _, err := Open(ctx, g.Dial, tenantID, g.limits(true))
	if err != nil {
		return Page{}, err
	}
	defer c.Close()
	cols, err := g.LoadColumns(ctx, c, b.Schema, b.Table)
	if err != nil {
		return Page{}, err
	}
	b.Columns = cols
	pageSQL, countSQL, args, err := BuildSelect(b)
	if err != nil {
		return Page{}, err
	}
	// count uses the WHERE args only (page args append limit, offset).
	countArgs := args[:len(args)-2]
	counted, err := c.Query(ctx, countSQL, countArgs...)
	if err != nil {
		return Page{}, err
	}
	total := int64(0)
	if len(counted.Data) == 1 {
		total, _ = asInt(counted.Data[0][0])
	}
	got, err := c.Query(ctx, pageSQL, args...)
	if err != nil {
		return Page{}, err
	}
	rows := make([]map[string]any, 0, len(got.Data))
	for _, rec := range got.Data {
		m := map[string]any{}
		for i, name := range got.Cols {
			if i < len(rec) {
				m[name] = rec[i]
			}
		}
		rows = append(rows, m)
	}
	limit := b.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return Page{Schema: b.Schema, Table: b.Table, Columns: cols, Rows: rows, Total: total, Limit: limit, Offset: b.Offset, Platform: PlatformTable(b.Table) && b.Schema == "public"}, nil
}

// Insert adds one row.
func (g *Gate) Insert(ctx context.Context, tenantID, schema, table string, values map[string]any, readOnly bool) (map[string]any, error) {
	if readOnly {
		return nil, ErrReadOnly
	}
	return g.mutate(ctx, tenantID, schema, table, readOnly, func(cols []Column) (string, []any, error) {
		return BuildInsert(schema, table, cols, values)
	})
}

// Update edits one row by primary key.
func (g *Gate) Update(ctx context.Context, tenantID, schema, table string, pk, values map[string]any, readOnly bool) (map[string]any, error) {
	if readOnly {
		return nil, ErrReadOnly
	}
	return g.mutate(ctx, tenantID, schema, table, readOnly, func(cols []Column) (string, []any, error) {
		return BuildUpdate(schema, table, cols, pk, values)
	})
}

// Delete removes one row. confirm must already have been checked by the caller
// and is checked again here.
func (g *Gate) Delete(ctx context.Context, tenantID, schema, table string, pk map[string]any, readOnly, confirm bool) error {
	if readOnly {
		return ErrReadOnly
	}
	if !confirm {
		return &ConfirmError{Kind: "destructive"}
	}
	c, t, err := Open(ctx, g.Dial, tenantID, g.limits(false))
	if err != nil {
		return err
	}
	defer c.Close()
	cols, err := g.LoadColumns(ctx, c, schema, table)
	if err != nil {
		return err
	}
	stmt, args, err := BuildDelete(schema, table, cols, pk)
	if err != nil {
		return err
	}
	if _, err := Classify(stmt, t); err != nil {
		return err
	}
	_, err = c.Exec(ctx, stmt, args...)
	return err
}

func (g *Gate) mutate(ctx context.Context, tenantID, schema, table string, readOnly bool, build func([]Column) (string, []any, error)) (map[string]any, error) {
	c, t, err := Open(ctx, g.Dial, tenantID, g.limits(readOnly))
	if err != nil {
		return nil, err
	}
	defer c.Close()
	cols, err := g.LoadColumns(ctx, c, schema, table)
	if err != nil {
		return nil, err
	}
	stmt, args, err := build(cols)
	if err != nil {
		return nil, err
	}
	class, err := Classify(stmt, t)
	if err != nil {
		return nil, err
	}
	if err := CheckRun(class, readOnly, true); err != nil {
		return nil, err
	}
	got, err := c.Query(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	if len(got.Data) == 0 {
		return map[string]any{}, nil
	}
	m := map[string]any{}
	for i, name := range got.Cols {
		if i < len(got.Data[0]) {
			m[name] = got.Data[0][i]
		}
	}
	return m, nil
}

// CreateTable creates a table in public from the type picker.
func (g *Gate) CreateTable(ctx context.Context, tenantID, name string, columns []NewColumn, readOnly bool) error {
	if readOnly {
		return ErrReadOnly
	}
	stmt, err := BuildCreateTable(name, columns)
	if err != nil {
		return err
	}
	_, err = g.Query(ctx, tenantID, stmt, false, false)
	return err
}

// AddColumn adds one column in public.
func (g *Gate) AddColumn(ctx context.Context, tenantID, table string, column NewColumn, readOnly bool) error {
	if readOnly {
		return ErrReadOnly
	}
	stmt, err := BuildAddColumn(table, column)
	if err != nil {
		return err
	}
	_, err = g.Query(ctx, tenantID, stmt, false, false)
	return err
}

// ApplyTemplate runs a built-in template as the tenant role.
func (g *Gate) ApplyTemplate(ctx context.Context, tenantID, id string, readOnly bool) error {
	if readOnly {
		return ErrReadOnly
	}
	tpl, ok := TemplateByID(id)
	if !ok {
		return fmtErrorf("that template doesn't exist")
	}
	for _, stmt := range tpl.Statements {
		if _, err := g.Query(ctx, tenantID, stmt, false, false); err != nil {
			return err
		}
	}
	return nil
}

func fmtErrorf(s string) error { return &plainError{s} }

type plainError struct{ s string }

func (e *plainError) Error() string { return e.s }
