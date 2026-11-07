-- +goose Up

CREATE TABLE trip_idempotency_keys (
    idempotency_key TEXT PRIMARY KEY,
    request_hash BYTEA NOT NULL,
    response_status INTEGER,
    response_body BYTEA,
    response_content_type TEXT,
    response_location TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT idempotency_key_length_check
        CHECK (char_length(idempotency_key) BETWEEN 1 AND 255),

    CONSTRAINT request_hash_length_check
        CHECK (octet_length(request_hash) = 32),

    CONSTRAINT response_status_check
        CHECK (
            response_status IS NULL
            OR response_status BETWEEN 100 AND 599
        )
);

-- +goose Down

DROP TABLE trip_idempotency_keys;