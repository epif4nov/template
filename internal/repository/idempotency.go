package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/epif4nov/template/internal/transaction"
)

var ErrIdempotencyConflict = errors.New(
	"idempotency key reused with different request",
)

// резервирует новый ключ или читает результат уже выполненного запроса
func (r *Repository) ClaimCreateTrip(
	ctx context.Context,
	key string,
	hash [32]byte,
) (bool, []byte, error) {
	db := transaction.Executor(ctx, r.pool)

	tag, err := db.Exec(ctx, `
		INSERT INTO trip_idempotency_keys
			(idempotency_key, request_hash)
		VALUES ($1, $2)
		ON CONFLICT (idempotency_key) DO NOTHING
	`, key, hash[:])
	if err != nil {
		return false, nil, fmt.Errorf(
			"reserve idempotency key: %w", err,
		)
	}

	if tag.RowsAffected() == 1 {
		return true, nil, nil
	}

	var savedHash []byte
	var savedBody []byte

	err = db.QueryRow(ctx, `
		SELECT request_hash, response_body
		FROM trip_idempotency_keys
		WHERE idempotency_key = $1
	`, key).Scan(&savedHash, &savedBody)
	if err != nil {
		return false, nil, fmt.Errorf(
			"load idempotency result: %w", err,
		)
	}

	if !bytes.Equal(savedHash, hash[:]) {
		return false, nil, ErrIdempotencyConflict
	}

	if len(savedBody) == 0 {
		return false, nil, errors.New(
			"idempotency result is incomplete",
		)
	}

	return false, savedBody, nil
}

//сохраняет снимок созданной поездки внутри той же транзакции.
func (r *Repository) CompleteCreateTrip(
	ctx context.Context,
	key string,
	trip Trip,
) error {
	body, err := json.Marshal(trip)
	if err != nil {
		return fmt.Errorf("marshal trip snapshot: %w", err)
	}

	location := "/api/v1/trips/" + trip.ID.String()

	db := transaction.Executor(ctx, r.pool)

	tag, err := db.Exec(ctx, `
		UPDATE trip_idempotency_keys
		SET
			response_status = 201,
			response_body = $2,
			response_content_type = 'application/json',
			response_location = $3
		WHERE idempotency_key = $1
		  AND response_status IS NULL
	`, key, body, location)
	if err != nil {
		return fmt.Errorf(
			"store idempotency result: %w", err,
		)
	}

	if tag.RowsAffected() != 1 {
		return errors.New(
			"idempotency result was not stored",
		)
	}

	return nil
}
