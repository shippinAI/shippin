package api

import (
	"errors"
	"net/http"
	"strings"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/tenantdb"
)

func (a *API) dbTables(w http.ResponseWriter, r *http.Request) {
	_, t, ok := a.dbMember(w, r)
	if !ok {
		return
	}
	tables, err := a.dbGate().Tables(r.Context(), t.ID)
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"tables": tables})
}

func (a *API) dbBrowse(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.dbMember(w, r)
	if !ok {
		return
	}
	schema, table, ok := a.dbRel(w, r)
	if !ok {
		return
	}
	b := tenantdb.Browse{Schema: schema, Table: table, Limit: queryInt(r, "limit", 50), Offset: queryInt(r, "offset", 0),
		Sort: r.URL.Query().Get("sort"), Dir: r.URL.Query().Get("dir")}
	if col := r.URL.Query().Get("column"); col != "" {
		b.Filters = []tenantdb.Filter{{Column: col, Op: r.URL.Query().Get("op"), Value: r.URL.Query().Get("value")}}
	}
	stmt := "browse " + schema + "." + table
	var page tenantdb.Page
	if !a.auditRun(w, r, auditBase(c, t, "browse", stmt, "read", true, false), func() (int, error) {
		var err error
		page, err = a.dbGate().Browse(r.Context(), t.ID, b)
		return len(page.Rows), err
	}) {
		return
	}
	writeJSON(w, page)
}

type columnIn struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Nullable   bool   `json:"nullable"`
	PrimaryKey bool   `json:"primary_key"`
	Default    string `json:"default"`
}

func (c columnIn) newColumn() tenantdb.NewColumn {
	return tenantdb.NewColumn{Name: c.Name, Type: c.Type, Nullable: c.Nullable, PrimaryKey: c.PrimaryKey, Default: c.Default}
}

func (a *API) dbCreateTable(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.dbMember(w, r)
	if !ok {
		return
	}
	var body struct {
		Name     string     `json:"name"`
		Columns  []columnIn `json:"columns"`
		ReadOnly bool       `json:"read_only"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	cols := make([]tenantdb.NewColumn, len(body.Columns))
	for i, col := range body.Columns {
		cols[i] = col.newColumn()
	}
	stmt, err := tenantdb.BuildCreateTable(body.Name, cols)
	if err != nil {
		auth.Problem(w, http.StatusBadRequest, err.Error())
		return
	}
	if !a.auditRun(w, r, auditBase(c, t, "create_table", stmt, "ddl", body.ReadOnly, false), func() (int, error) {
		return 0, a.dbGate().CreateTable(r.Context(), t.ID, body.Name, cols, body.ReadOnly)
	}) {
		return
	}
	writeJSON(w, map[string]any{"name": body.Name, "schema": "public"})
}

func (a *API) dbAddColumn(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.dbMember(w, r)
	if !ok {
		return
	}
	table, err := dbTable(r)
	if err != nil {
		auth.Problem(w, http.StatusBadRequest, err.Error())
		return
	}
	var body struct {
		columnIn
		ReadOnly bool `json:"read_only"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	stmt, err := tenantdb.BuildAddColumn(table, body.newColumn())
	if err != nil {
		auth.Problem(w, http.StatusBadRequest, err.Error())
		return
	}
	if !a.auditRun(w, r, auditBase(c, t, "add_column", stmt, "ddl", body.ReadOnly, false), func() (int, error) {
		return 0, a.dbGate().AddColumn(r.Context(), t.ID, table, body.newColumn(), body.ReadOnly)
	}) {
		return
	}
	writeJSON(w, map[string]any{"table": table, "column": body.Name})
}

func (a *API) dbInsertRow(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.dbMember(w, r)
	if !ok {
		return
	}
	schema, table, ok := a.dbRel(w, r)
	if !ok {
		return
	}
	var body struct {
		Values   cellMap `json:"values"`
		ReadOnly bool    `json:"read_only"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	var row map[string]any
	stmt := "insert into " + schema + "." + table + " (" + strings.Join(mapKeys(body.Values), ", ") + ")"
	if !a.auditRun(w, r, auditBase(c, t, "insert", stmt, "write", body.ReadOnly, false), func() (int, error) {
		var err error
		row, err = a.dbGate().Insert(r.Context(), t.ID, schema, table, body.Values, body.ReadOnly)
		if err != nil {
			return 0, err
		}
		return 1, nil
	}) {
		return
	}
	writeJSON(w, map[string]any{"row": row})
}

func (a *API) dbUpdateRow(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.dbMember(w, r)
	if !ok {
		return
	}
	schema, table, ok := a.dbRel(w, r)
	if !ok {
		return
	}
	var body struct {
		PK       cellMap `json:"pk"`
		Values   cellMap `json:"values"`
		ReadOnly bool    `json:"read_only"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	var row map[string]any
	stmt := "update " + schema + "." + table
	if !a.auditRun(w, r, auditBase(c, t, "update", stmt, "write", body.ReadOnly, false), func() (int, error) {
		var err error
		row, err = a.dbGate().Update(r.Context(), t.ID, schema, table, body.PK, body.Values, body.ReadOnly)
		if err != nil {
			return 0, err
		}
		return 1, nil
	}) {
		return
	}
	writeJSON(w, map[string]any{"row": row})
}

func (a *API) dbDeleteRow(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.dbMember(w, r)
	if !ok {
		return
	}
	schema, table, ok := a.dbRel(w, r)
	if !ok {
		return
	}
	var body struct {
		PK       cellMap `json:"pk"`
		ReadOnly bool    `json:"read_only"`
		Confirm  bool    `json:"confirm"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	stmt := "delete from " + schema + "." + table
	entry := auditBase(c, t, "delete", stmt, "destructive", body.ReadOnly, body.Confirm)
	if body.ReadOnly {
		a.auditReject(w, r, entry, tenantdb.ErrReadOnly)
		return
	}
	if !body.Confirm {
		a.auditReject(w, r, entry, &tenantdb.ConfirmError{Kind: "destructive"})
		return
	}
	if !a.auditRun(w, r, entry, func() (int, error) {
		return 1, a.dbGate().Delete(r.Context(), t.ID, schema, table, body.PK, body.ReadOnly, body.Confirm)
	}) {
		return
	}
	writeJSON(w, map[string]any{"deleted": true})
}

func (a *API) dbQuery(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.dbMember(w, r)
	if !ok {
		return
	}
	var body struct {
		SQL      string `json:"sql"`
		ReadOnly bool   `json:"read_only"`
		Confirm  bool   `json:"confirm"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	target, err := tenantdb.TargetOf(t.ID)
	if err != nil {
		writeDBErr(w, err)
		return
	}
	class, err := tenantdb.Classify(body.SQL, target)
	entry := auditBase(c, t, "query", body.SQL, "", body.ReadOnly, body.Confirm)
	if err != nil {
		a.auditReject(w, r, entry, err)
		return
	}
	entry.Kind = class.Kind
	if err := tenantdb.CheckRun(class, body.ReadOnly, body.Confirm); err != nil {
		a.auditReject(w, r, entry, err)
		return
	}
	var res tenantdb.Result
	if !a.auditRun(w, r, entry, func() (int, error) {
		var err error
		res, err = a.dbGate().Query(r.Context(), t.ID, body.SQL, body.ReadOnly, body.Confirm)
		return res.RowCount, err
	}) {
		return
	}
	writeJSON(w, res)
}

func (a *API) dbApplyTemplate(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.dbMember(w, r)
	if !ok {
		return
	}
	id := r.PathValue("templateId")
	tpl, found := tenantdb.TemplateByID(id)
	if !found {
		auth.Problem(w, http.StatusNotFound, "That template doesn't exist.")
		return
	}
	var body struct {
		ReadOnly bool `json:"read_only"`
	}
	if r.ContentLength != 0 {
		if !decodeBody(w, r, &body) {
			return
		}
	}
	stmt := "template " + tpl.ID + ": " + strings.Join(tpl.Tables, ", ")
	if !a.auditRun(w, r, auditBase(c, t, "template", stmt, "ddl", body.ReadOnly, false), func() (int, error) {
		return len(tpl.Statements), a.dbGate().ApplyTemplate(r.Context(), t.ID, id, body.ReadOnly)
	}) {
		return
	}
	writeJSON(w, map[string]any{"template": tpl.ID, "tables": tpl.Tables})
}

func (a *API) dbRel(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	schema, err := dbSchema(r)
	if err != nil {
		auth.Problem(w, http.StatusBadRequest, err.Error())
		return "", "", false
	}
	table, err := dbTable(r)
	if err != nil {
		auth.Problem(w, http.StatusBadRequest, err.Error())
		return "", "", false
	}
	return schema, table, true
}

func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func (a *API) dbListSnippets(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.dbMember(w, r)
	if !ok {
		return
	}
	store := a.snippetDB()
	if store == nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Saved snippets aren't available.")
		return
	}
	items, err := store.ListSnippets(r.Context(), t.ID, c.id.Sub)
	if err != nil {
		auth.Problem(w, http.StatusBadGateway, "Couldn't load saved snippets.")
		return
	}
	writeJSON(w, map[string]any{"snippets": items})
}

func (a *API) dbSaveSnippet(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.dbMember(w, r)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
		SQL  string `json:"sql"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || len(body.Name) > 80 || strings.TrimSpace(body.SQL) == "" || len(body.SQL) > 20_000 {
		auth.Problem(w, http.StatusBadRequest, "Give the snippet a short name and one statement.")
		return
	}
	store := a.snippetDB()
	if store == nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Saved snippets aren't available.")
		return
	}
	saved, err := store.SaveSnippet(r.Context(), db.SQLSnippet{TenantID: t.ID, Owner: c.id.Sub, Name: body.Name, SQL: body.SQL})
	if err != nil {
		auth.Problem(w, http.StatusBadGateway, "Couldn't save that snippet.")
		return
	}
	writeJSON(w, saved)
}

func (a *API) dbDeleteSnippet(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.dbMember(w, r)
	if !ok {
		return
	}
	store := a.snippetDB()
	if store == nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Saved snippets aren't available.")
		return
	}
	if err := store.DeleteSnippet(r.Context(), t.ID, c.id.Sub, r.PathValue("snippetId")); err != nil {
		auth.Problem(w, http.StatusNotFound, "That snippet isn't yours, or it was already removed.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) dbAudit(w http.ResponseWriter, r *http.Request) {
	_, t, ok := a.dbMember(w, r)
	if !ok {
		return
	}
	log := a.logger()
	if log == nil {
		auth.Problem(w, http.StatusServiceUnavailable, tenantdb.Plain(tenantdb.ErrAudit))
		return
	}
	items, err := log.ListSQLAudit(r.Context(), t.ID, 50)
	if err != nil {
		auth.Problem(w, http.StatusBadGateway, "Couldn't load the SQL audit log.")
		return
	}
	if items == nil {
		items = []db.SQLAudit{}
	}
	writeJSON(w, map[string]any{"entries": items})
}

func (a *API) dbBackups(w http.ResponseWriter, r *http.Request) {
	_, t, ok := a.dbMember(w, r)
	if !ok {
		return
	}
	items, err := a.backupDB().List(r.Context(), t.ID)
	if err != nil {
		auth.Problem(w, http.StatusBadGateway, "Couldn't load backups.")
		return
	}
	writeJSON(w, map[string]any{
		"backups": items,
		"backend": "pending",
		"message": "One-click backup will dump this database as your database role. The backup service isn't connected yet, so requests are recorded and nothing is copied.",
	})
}

func (a *API) dbCreateBackup(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.dbMember(w, r)
	if !ok {
		return
	}
	var body struct {
		Note string `json:"note"`
	}
	if r.ContentLength != 0 {
		if !decodeBody(w, r, &body) {
			return
		}
	}
	if len(body.Note) > 200 {
		auth.Problem(w, http.StatusBadRequest, "Keep the note short.")
		return
	}
	b, err := a.backupDB().Create(r.Context(), t.ID, c.id.Sub, strings.TrimSpace(body.Note))
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeStatus(w, http.StatusAccepted, map[string]any{
		"backup":  b,
		"message": "Recorded. No dump was taken — the Postgres backup service is still to be connected.",
	})
}

func (a *API) dbRestoreBackup(w http.ResponseWriter, r *http.Request) {
	_, t, ok := a.dbMember(w, r)
	if !ok {
		return
	}
	err := a.backupDB().Restore(r.Context(), t.ID, r.PathValue("backupId"))
	if err == nil {
		auth.Problem(w, http.StatusNotImplemented, "Restore is not available yet.")
		return
	}
	if errors.Is(err, tenantdb.ErrBackupNotFound) {
		auth.Problem(w, http.StatusNotFound, "That backup isn't in this organization.")
		return
	}
	if errors.Is(err, tenantdb.ErrBackupPending) {
		auth.Problem(w, http.StatusNotImplemented, "Restore isn't connected to Postgres yet. Nothing was changed.")
		return
	}
	auth.Problem(w, http.StatusBadGateway, "Couldn't restore that backup.")
}
