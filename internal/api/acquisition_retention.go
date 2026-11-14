package api

import (
	"context"
	"log"
	"time"

	"shippin.cloud/console/internal/acquisition"
)

const (
	acquisitionRetentionInterval = 5 * time.Minute
	acquisitionRetentionRetry    = time.Second
	acquisitionRetentionBatch    = 500
	acquisitionRetentionBatches  = 20
)

// RunAcquisitionRetention purges expired encrypted lead payloads immediately
// and periodically. Tombstone revision floors are intentionally permanent.
func (a *API) RunAcquisitionRetention(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		more := a.purgeAcquisitionRetention(ctx)
		delay := acquisitionRetentionInterval
		if more {
			delay = acquisitionRetentionRetry
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// purgeAcquisitionRetention returns true when a bounded run filled its batches
// and should continue after a short delay. Expired PII is normally removed at
// the next five-minute pass; larger backlogs drain in bounded 10,000-row cycles
// with a one-second pause. If the process is stopped, purge resumes at startup.
func (a *API) purgeAcquisitionRetention(parent context.Context) bool {
	if a == nil || a.db == nil || a.db.Pool == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	repo := acquisition.PostgresRepository{}
	for i := 0; i < acquisitionRetentionBatches; i++ {
		more, err := repo.PurgeExpired(ctx, a.db.Pool, time.Now().UTC(), acquisitionRetentionBatch)
		if err != nil {
			// The repository deliberately sanitizes errors; avoid logging DB details.
			log.Print("acquisition retention: expired lead purge failed")
			return false
		}
		if !more {
			return false
		}
		if ctx.Err() != nil {
			return true
		}
	}
	log.Print("acquisition retention: bounded purge remains for the next run")
	return true
}
