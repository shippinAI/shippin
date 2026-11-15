package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/ports"
)

// Fresh is how long a read may be served from memory. A save in this process clears it at once;
// another process sees the change within this time.
const Fresh = 10 * time.Second

// keyFresh is how long a key's value is kept in memory after it is read from the store.
const keyFresh = 5 * time.Minute

// pendingFor is how long a key written for a save that hasn't committed is kept, before the sweep
// deletes it as a save that failed.
const pendingFor = 15 * time.Minute

// Store is the settings of this installation: the sections, their saved rows and the file they seed
// from. It is safe for concurrent use.
type Store struct {
	pool     *pgxpool.Pool
	file     *config.Config
	sections []Section
	secrets  func() ports.Secrets

	mu       sync.Mutex
	rows     map[string]row
	loadedAt time.Time
	keys     map[ports.SecretRef]cachedKey
}

type row struct {
	Values    Values
	Keys      map[string]ports.SecretRef
	Version   int
	UpdatedBy string
	UpdatedAt time.Time
}

type cachedKey struct {
	value string
	at    time.Time
}

// New is the store for these sections. file is the bootstrap configuration they seed from; secrets
// is the secret store, asked for each time so that connecting one takes effect at once.
func New(pool *pgxpool.Pool, file *config.Config, sections []Section, secrets func() ports.Secrets) (*Store, error) {
	seen := map[string]bool{}
	for _, s := range sections {
		if err := s.Validate(); err != nil {
			return nil, err
		}
		if seen[s.Key] {
			return nil, fmt.Errorf("section %s is declared twice", s.Key)
		}
		seen[s.Key] = true
	}
	if file == nil {
		file = &config.Config{}
	}
	ss := slices.Clone(sections)
	slices.SortFunc(ss, func(a, b Section) int { return strings.Compare(a.Title, b.Title) })
	return &Store{pool: pool, file: file, sections: ss, secrets: secrets, keys: map[ports.SecretRef]cachedKey{}}, nil
}

// Sections are every section, by title.
func (s *Store) Sections() []Section { return s.sections }

// Section is the section with key.
func (s *Store) Section(key string) (Section, bool) {
	for _, sec := range s.sections {
		if sec.Key == key {
			return sec, true
		}
	}
	return Section{}, false
}

// Invalidate forgets what was read, so the next read goes to the database and the secret store.
func (s *Store) Invalidate() {
	s.mu.Lock()
	s.rows, s.keys = nil, map[ports.SecretRef]cachedKey{}
	s.mu.Unlock()
}

func (s *Store) load(ctx context.Context) (map[string]row, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rows != nil && time.Since(s.loadedAt) < Fresh {
		return s.rows, nil
	}
	if s.pool == nil {
		return map[string]row{}, nil
	}
	rs, err := s.pool.Query(ctx, `SELECT section, vals, keys, version, updated_by, updated_at FROM shippin_settings`)
	if err != nil {
		return nil, err
	}
	out := map[string]row{}
	for rs.Next() {
		var key string
		var r row
		if err := rs.Scan(&key, &r.Values, &r.Keys, &r.Version, &r.UpdatedBy, &r.UpdatedAt); err != nil {
			rs.Close()
			return nil, err
		}
		out[key] = r
	}
	if err := rs.Err(); err != nil {
		return nil, err
	}
	s.rows, s.loadedAt = out, time.Now()
	return out, nil
}

// seed is the section as the file gives it.
func (s *Store) seed(sec Section) Values {
	v := Values{}
	if raw, err := json.Marshal(sec.Seed(s.file)); err == nil {
		_ = json.Unmarshal(raw, &v)
	}
	out := Values{}
	for _, f := range sec.Fields {
		if x, ok := v[f.Name]; ok && !f.Secret() {
			out[f.Name] = x
		}
	}
	return out
}

func (s *Store) seedKeys(sec Section) map[string]string {
	if sec.SeedKeys == nil {
		return nil
	}
	return sec.SeedKeys(s.file)
}

// current is a section's values and keys in force: the saved row, else the file.
func (s *Store) current(ctx context.Context, sec Section) (Values, row, bool, error) {
	rows, err := s.load(ctx)
	if err != nil {
		return nil, row{}, false, err
	}
	r, saved := rows[sec.Key]
	v := s.seed(sec)
	if saved {
		for _, f := range sec.Fields {
			if x, ok := r.Values[f.Name]; ok && !f.Secret() {
				v[f.Name] = x
			}
		}
	}
	return v, r, saved, nil
}

// Values is a section's values in force and its version (0 while it comes from the file).
func (s *Store) Values(ctx context.Context, key string) (Values, int, error) {
	sec, ok := s.Section(key)
	if !ok {
		return nil, 0, fmt.Errorf("settings: no section %s", key)
	}
	v, r, _, err := s.current(ctx, sec)
	return v, r.Version, err
}

// Get is a section decoded into T, and its version. A database that can't be read gives the file's
// values, so a panel that loses its database for a moment keeps its last known configuration shape.
func Get[T any](ctx context.Context, s *Store, key string) (T, int) {
	v, version, err := s.Values(ctx, key)
	if err != nil {
		log.Printf("settings %s: %v (using the file)", key, err)
		sec, _ := s.Section(key)
		v = s.seed(sec)
	}
	t, err := As[T](v)
	if err != nil {
		log.Printf("settings %s: %v", key, err)
	}
	return t, version
}

// Key is the value of a section's key field: from the secret store once the section holds it there,
// else from the environment while the section has never been saved in the panel. "" means none.
func (s *Store) Key(ctx context.Context, section, field string) (string, error) {
	sec, ok := s.Section(section)
	if !ok {
		return "", fmt.Errorf("settings: no section %s", section)
	}
	_, r, saved, err := s.current(ctx, sec)
	if err != nil {
		return "", err
	}
	if !saved {
		return s.seedKeys(sec)[field], nil
	}
	ref, ok := r.Keys[field]
	if !ok {
		return "", nil
	}
	s.mu.Lock()
	c, hit := s.keys[ref]
	s.mu.Unlock()
	if hit && time.Since(c.at) < keyFresh {
		return c.value, nil
	}
	store := s.secrets()
	if store == nil {
		return "", ports.ErrNoSecretStore
	}
	value, err := store.Read(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("settings %s: reading %s from the secret store: %w", section, field, err)
	}
	s.mu.Lock()
	s.keys[ref] = cachedKey{value, time.Now()}
	s.mu.Unlock()
	return value, nil
}

// View is a section as the panel shows it: its fields, the values in force and where each comes from.
type View struct {
	Key     string      `json:"key"`
	Title   string      `json:"title"`
	Summary string      `json:"summary"`
	Fields  []FieldView `json:"fields"`
	// Version is the saved version; 0 while the section still comes from the bootstrap file.
	Version   int        `json:"version"`
	Source    string     `json:"source"`
	UpdatedBy string     `json:"updated_by,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// FieldView is one field with its value in force, or for a key whether one is saved and where.
type FieldView struct {
	Field
	Value     json.RawMessage `json:"value,omitempty"`
	FileValue json.RawMessage `json:"file_value,omitempty"`
	Key       *KeyState       `json:"key,omitempty"`
}

// KeyState says whether a key is saved and where, never what it is.
type KeyState struct {
	Saved bool `json:"saved"`
	// In is "secret_store", "environment" or "" (none).
	In   string `json:"in"`
	Name string `json:"name,omitempty"`
}

// View is the section with key, as the panel shows it.
func (s *Store) View(ctx context.Context, key string) (View, error) {
	sec, ok := s.Section(key)
	if !ok {
		return View{}, mcpl.NotFound("There is no settings section " + key + ".")
	}
	v, r, saved, err := s.current(ctx, sec)
	if err != nil {
		return View{}, err
	}
	file, env := s.seed(sec), s.seedKeys(sec)
	out := View{Key: sec.Key, Title: sec.Title, Summary: sec.Summary, Source: "file"}
	if saved {
		at := r.UpdatedAt
		out.Version, out.Source, out.UpdatedBy, out.UpdatedAt = r.Version, "panel", r.UpdatedBy, &at
	}
	for _, f := range sec.Fields {
		fv := FieldView{Field: f}
		if f.Secret() {
			ks := &KeyState{}
			if ref, ok := r.Keys[f.Name]; saved && ok {
				ks.Saved, ks.In, ks.Name = true, "secret_store", ref.Path+"/"+ref.Name
			} else if !saved && env[f.Name] != "" {
				ks.Saved, ks.In = true, "environment"
			}
			fv.Key = ks
		} else {
			fv.Value, _ = json.Marshal(v[f.Name])
			if x, ok := file[f.Name]; ok {
				fv.FileValue, _ = json.Marshal(x)
			}
		}
		out.Fields = append(out.Fields, fv)
	}
	return out, nil
}

// Change is one save of a section.
type Change struct {
	Section string
	// Base is the version the change was made from; a save based on an older one is refused.
	Base   int
	Values map[string]json.RawMessage
	// Keys are new key values by field; null removes the key.
	Keys map[string]*string
	// Import moves these keys from the environment into the secret store.
	Import []string
	By     string
	Reason string
}

type checking struct {
	s    *Store
	ctx  context.Context
	keys map[string]bool
}

func (c checking) Has(name string) bool { return c.keys[name] }
func (c checking) Section(key string) Values {
	v, _, err := c.s.Values(c.ctx, key)
	if err != nil {
		return Values{}
	}
	return v
}

// Save validates a change, writes its new keys to the secret store and saves the section on q, the
// change's transaction, with its history. Replaced keys are queued for deletion in the same
// transaction; Sweep deletes them once it has committed.
func (s *Store) Save(ctx context.Context, q mcpl.DB, ch Change) (View, error) {
	sec, ok := s.Section(ch.Section)
	if !ok {
		return View{}, mcpl.NotFound("There is no settings section " + ch.Section + ".")
	}
	var cur row
	var keysRaw []byte
	err := q.QueryRow(ctx, `SELECT vals, keys, version FROM shippin_settings WHERE section=$1 FOR UPDATE`, sec.Key).Scan(&cur.Values, &keysRaw, &cur.Version)
	saved := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return View{}, err
	}
	if saved {
		_ = json.Unmarshal(keysRaw, &cur.Keys)
	}
	if ch.Base != cur.Version {
		return View{}, mcpl.Conflict("This section changed since you opened it. Reload it to see the change, then save again.")
	}
	values := s.seed(sec)
	if saved {
		for k, x := range cur.Values {
			values[k] = x
		}
	}
	for name, raw := range ch.Values {
		f, ok := sec.field(name)
		if !ok {
			return View{}, mcpl.Invalid(sec.Title + " has no field " + name + ".")
		}
		if f.Secret() {
			return View{}, mcpl.Invalid(f.Label + " is a key: send it in keys, not values.")
		}
		v, err := f.check(raw)
		if err != nil {
			return View{}, mcpl.Invalid(err.Error())
		}
		values[name] = v
	}

	// The keys after this change: what the section holds, or on its first save what the environment
	// gives (moved into the store only when asked), then the change's own.
	refs := map[string]ports.SecretRef{}
	for k, r := range cur.Keys {
		refs[k] = r
	}
	writes := map[string]string{}
	var removed []string
	env := s.seedKeys(sec)
	for _, name := range ch.Import {
		f, ok := sec.field(name)
		if !ok || !f.Secret() {
			return View{}, mcpl.Invalid(sec.Title + " has no key " + name + ".")
		}
		if saved || env[name] == "" {
			return View{}, mcpl.Invalid(f.Label + " isn't in the environment to move.")
		}
		writes[name] = env[name]
	}
	for name, v := range ch.Keys {
		f, ok := sec.field(name)
		if !ok || !f.Secret() {
			return View{}, mcpl.Invalid(sec.Title + " has no key " + name + ".")
		}
		if v == nil {
			removed = append(removed, name)
			delete(writes, name)
			continue
		}
		value := *v
		if f.Kind == Key {
			value = strings.TrimSpace(value)
		}
		switch {
		case value == "":
			return View{}, mcpl.Invalid(f.Label + ": enter the key, or remove it.")
		case len(value) > 64<<10:
			return View{}, mcpl.Invalid(f.Label + " is longer than 64 KiB.")
		case f.Kind == Key && strings.IndexFunc(value, unicode.IsControl) >= 0:
			return View{}, mcpl.Invalid(f.Label + " must be one line.")
		}
		if f.Valid != nil {
			if err := f.Valid(value); err != nil {
				return View{}, mcpl.Invalid(f.Label + ": " + err.Error())
			}
		}
		writes[name] = value
	}
	has := map[string]bool{}
	for k := range refs {
		has[k] = true
	}
	for k := range writes {
		has[k] = true
	}
	for _, k := range removed {
		delete(has, k)
	}
	if sec.Check != nil {
		if err := sec.Check(values, checking{s, ctx, has}); err != nil {
			return View{}, mcpl.Invalid(sec.Title + ": " + err.Error())
		}
	}

	version := cur.Version + 1
	var retire []ports.SecretRef
	changed := []string{}
	for _, k := range removed {
		if r, ok := refs[k]; ok {
			retire = append(retire, r)
			delete(refs, k)
		}
		changed = append(changed, k)
	}
	if len(writes) > 0 {
		store := s.secrets()
		if store == nil {
			return View{}, mcpl.Conflict(ports.ErrNoSecretStore.Error())
		}
		where, err := store.Where(ctx)
		if err != nil {
			return View{}, mcpl.Conflict(err.Error())
		}
		names := make([]string, 0, len(writes))
		for k := range writes {
			names = append(names, k)
		}
		slices.Sort(names)
		for _, k := range names {
			ref := where
			ref.Path = strings.TrimRight(where.Path, "/") + "/" + sec.Key
			ref.Name = fmt.Sprintf("%s_V%d", strings.ToUpper(k), version)
			// Queued for deletion before it exists, on its own connection: if this save never commits,
			// the sweep removes the key. The save takes it off the queue when it commits.
			var pending int64
			if err := s.pool.QueryRow(ctx, `INSERT INTO shippin_setting_retired_secrets(ref, due_at) VALUES ($1, now() + $2::interval) RETURNING id`,
				ref, fmt.Sprintf("%d seconds", int(pendingFor.Seconds()))).Scan(&pending); err != nil {
				return View{}, err
			}
			if err := store.Write(ctx, ref, writes[k]); err != nil {
				return View{}, &mcpl.Error{Status: 502, Message: "Vaultix didn't save " + k + ": " + err.Error()}
			}
			if _, err := q.Exec(ctx, `DELETE FROM shippin_setting_retired_secrets WHERE id=$1`, pending); err != nil {
				return View{}, err
			}
			if old, ok := refs[k]; ok {
				retire = append(retire, old)
			}
			refs[k] = ref
			changed = append(changed, k)
		}
	}
	slices.Sort(changed)
	changed = slices.Compact(changed)
	stored := Values{}
	for _, f := range sec.Fields {
		if x, ok := values[f.Name]; ok && !f.Secret() {
			stored[f.Name] = x
		}
	}
	// A first save inserts, so two first saves can't both win; a later one updates the version it read.
	if saved {
		_, err = q.Exec(ctx, `UPDATE shippin_settings SET vals=$2, keys=$3, version=$4, updated_by=$5, updated_at=now() WHERE section=$1`,
			sec.Key, stored, refs, version, ch.By)
	} else {
		_, err = q.Exec(ctx, `INSERT INTO shippin_settings(section, vals, keys, version, updated_by) VALUES ($1,$2,$3,$4,$5)`,
			sec.Key, stored, refs, version, ch.By)
	}
	if pe := (*pgconn.PgError)(nil); errors.As(err, &pe) && pe.Code == "23505" {
		return View{}, mcpl.Conflict("This section was saved by someone else just now. Reload it, then save again.")
	}
	if err != nil {
		return View{}, err
	}
	if _, err := q.Exec(ctx, `INSERT INTO shippin_setting_versions(section, version, vals, keys_changed, changed_by, reason) VALUES ($1,$2,$3,$4,$5,$6)`,
		sec.Key, version, stored, changed, ch.By, ch.Reason); err != nil {
		return View{}, err
	}
	for _, r := range retire {
		if _, err := q.Exec(ctx, `INSERT INTO shippin_setting_retired_secrets(ref) VALUES ($1)`, r); err != nil {
			return View{}, err
		}
	}
	// The saved view, as the transaction sees it.
	s.Invalidate()
	at := time.Now()
	out := View{Key: sec.Key, Title: sec.Title, Summary: sec.Summary, Version: version, Source: "panel", UpdatedBy: ch.By, UpdatedAt: &at}
	file := s.seed(sec)
	for _, f := range sec.Fields {
		fv := FieldView{Field: f}
		if f.Secret() {
			ks := &KeyState{}
			if r, ok := refs[f.Name]; ok {
				ks.Saved, ks.In, ks.Name = true, "secret_store", r.Path+"/"+r.Name
			}
			fv.Key = ks
		} else {
			fv.Value, _ = json.Marshal(stored[f.Name])
			if x, ok := file[f.Name]; ok {
				fv.FileValue, _ = json.Marshal(x)
			}
		}
		out.Fields = append(out.Fields, fv)
	}
	return out, nil
}

// Sweep deletes the keys that are due from the secret store: replaced, removed, or written for a save
// that never committed. One that can't be deleted is tried again later. It returns how many went.
func (s *Store) Sweep(ctx context.Context) (int, error) {
	if s.pool == nil {
		return 0, nil
	}
	store := s.secrets()
	if store == nil {
		return 0, nil
	}
	rs, err := s.pool.Query(ctx, `SELECT id, ref FROM shippin_setting_retired_secrets WHERE due_at <= now() ORDER BY id LIMIT 50`)
	if err != nil {
		return 0, err
	}
	type due struct {
		id  int64
		ref ports.SecretRef
	}
	var items []due
	for rs.Next() {
		var d due
		if err := rs.Scan(&d.id, &d.ref); err != nil {
			rs.Close()
			return 0, err
		}
		items = append(items, d)
	}
	if err := rs.Err(); err != nil {
		return 0, err
	}
	n := 0
	var errs []error
	for _, d := range items {
		if err := store.Delete(ctx, d.ref); err != nil {
			errs = append(errs, err)
			_, _ = s.pool.Exec(ctx, `UPDATE shippin_setting_retired_secrets SET attempts=attempts+1, last_error=$2,
			  due_at=now() + least(interval '1 hour', interval '1 minute' * power(2, attempts)) WHERE id=$1`, d.id, trimErr(err))
			continue
		}
		if _, err := s.pool.Exec(ctx, `DELETE FROM shippin_setting_retired_secrets WHERE id=$1`, d.id); err != nil {
			errs = append(errs, err)
			continue
		}
		n++
	}
	return n, errors.Join(errs...)
}

// Run sweeps retired keys every minute until ctx ends.
func (s *Store) Run(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		if n, err := s.Sweep(ctx); err != nil {
			log.Printf("settings: removing replaced keys: %v", err)
		} else if n > 0 {
			log.Printf("settings: removed %d replaced keys from the secret store", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func trimErr(err error) string {
	m := err.Error()
	if len(m) > 500 {
		m = m[:500]
	}
	return m
}
