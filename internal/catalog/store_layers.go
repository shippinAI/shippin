package catalog

import (
	"context"
	"encoding/json"
)

// Defaults lists layered settings, optionally for one layer/ref.
func (s *Store) Defaults(ctx context.Context, layer, ref string) (json.RawMessage, error) {
	return s.listJSON(ctx, `SELECT id, layer, ref, key, value, lock, bounds, note, updated_at FROM tenant_defaults
		WHERE ($1='' OR layer=$1) AND ($2='' OR ref=$2) ORDER BY key, layer`, layer, ref)
}

// PutDefault upserts one layered setting.
func (s *Store) PutDefault(ctx context.Context, it Item) error {
	var bounds any
	if it.Bounds != nil {
		bounds = it.Bounds
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO tenant_defaults(layer,ref,key,value,lock,bounds,note) VALUES($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (layer,ref,key) DO UPDATE SET value=$4, lock=$5, bounds=$6, note=$7, updated_at=now()`,
		it.Layer, it.Ref, it.Key, it.Value, lockOr(it.Lock), bounds, it.Note)
	return err
}

func (s *Store) DeleteDefault(ctx context.Context, layer, ref, key string) error {
	return s.exec(ctx, `DELETE FROM tenant_defaults WHERE layer=$1 AND ref=$2 AND key=$3`, layer, ref, key)
}
