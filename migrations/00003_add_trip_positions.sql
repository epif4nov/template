-- +goose Up

-- Добавляем время последней координаты поездки
ALTER TABLE trips
    ADD COLUMN last_position_at TIMESTAMPTZ;

-- Таблица координат маршрута
CREATE TABLE trip_positions (
    id BIGSERIAL PRIMARY KEY,
    trip_id UUID NOT NULL
        REFERENCES trips(id) ON DELETE CASCADE,

    latitude DOUBLE PRECISION NOT NULL
        CHECK (latitude BETWEEN -90 AND 90),

    longitude DOUBLE PRECISION NOT NULL
        CHECK (longitude BETWEEN -180 AND 180),

    recorded_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Быстрое получение маршрута в хронологическом порядке
CREATE INDEX trip_positions_trip_recorded_idx
    ON trip_positions (trip_id, recorded_at, id);

-- Поиск активных поездок без свежих координат
CREATE INDEX trips_active_position_check_idx
    ON trips (
        (COALESCE(last_position_at, started_at)),
        id
    )
    WHERE status = 'active';

-- +goose Down
DROP INDEX IF EXISTS trips_active_position_check_idx;
DROP TABLE IF EXISTS trip_positions;

ALTER TABLE trips
    DROP COLUMN IF EXISTS last_position_at;