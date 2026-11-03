package subs

import "context"

// UtilityFailed atomically records uncertain physical state for immediate cleanup.
func (s *Store) UtilityFailed(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE ai_homes SET utility_state='error', state='error',
	  error='the AI terminal needs cleanup', updated_at=now() WHERE id=$1`, id)
	return err
}

// TouchUtility cannot overwrite a concurrent failure or stop with stale caller state.
func (s *Store) TouchUtility(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE ai_homes SET utility_seen=now(), updated_at=now() WHERE id=$1`, id)
	return err
}

// UtilityStopped clears only the cleanup error. Volume readiness must be
// reverified on activation; a stopped/absent container does not prove it.
func (s *Store) UtilityStopped(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE ai_homes SET utility_state='stopped',
	  state=CASE WHEN error='the AI terminal needs cleanup' THEN 'creating' ELSE state END,
	  error=CASE WHEN error='the AI terminal needs cleanup' THEN NULL ELSE error END,
	  updated_at=now() WHERE id=$1`, id)
	return err
}
