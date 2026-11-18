-- +goose NO TRANSACTION
-- +goose Up
-- INCLUDE permits reading driver_id from the index for the polling query.
-- The table already contains trips, so build and drop indexes concurrently.
CREATE INDEX CONCURRENTLY IF NOT EXISTS trips_active_stale_lookup_idx
    ON trips ((COALESCE(last_position_at, started_at)), id)
    INCLUDE (driver_id)
    WHERE status = 'active';
DROP INDEX CONCURRENTLY IF EXISTS trips_active_position_check_idx;

-- +goose Down
CREATE INDEX CONCURRENTLY IF NOT EXISTS trips_active_position_check_idx
    ON trips ((COALESCE(last_position_at, started_at)), id)
    WHERE status = 'active';
DROP INDEX CONCURRENTLY IF EXISTS trips_active_stale_lookup_idx;
