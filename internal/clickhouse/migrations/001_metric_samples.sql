-- One reading of one metric for a host (guest_id '') or a guest, as its provider reported it (OBS-01).
-- Missing readings are absent rows, never zeros. Retention is the table TTL; the panel setting changes it.
CREATE TABLE IF NOT EXISTS shippin_metric_samples
(
    at        DateTime64(3, 'UTC') CODEC(Delta, ZSTD),
    server_id UUID,
    guest_id  String,
    node      LowCardinality(String),
    metric    LowCardinality(String),
    value     Float64 CODEC(Gorilla, ZSTD)
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(at)
ORDER BY (server_id, guest_id, metric, at)
TTL toDateTime(at) + INTERVAL 30 DAY DELETE
