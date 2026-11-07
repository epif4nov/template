package transaction

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TxManager — интерфейс менеджера транзакций
type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

// Manager управляет транзакциями PostgreSQL
type Manager struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

// Собственный тип ключа для хранения транзакции в context
type txContextKey struct{}

// NewManager создаёт менеджер транзакций
func NewManager(
	pool *pgxpool.Pool,
	timeout time.Duration,
) *Manager {
	return &Manager{
		pool:    pool,
		timeout: timeout,
	}
}

// Do выполняет функцию внутри транзакции
func (m *Manager) Do(
	ctx context.Context,
	fn func(ctx context.Context) error,
) (err error) {

	// Если транзакция уже существует, повторно её не открываем
	if _, ok := ctx.Value(txContextKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}

	// Создаём контекст с ограниченным временем выполнения.
	txCtx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	// Открываем транзакцию с уровнем изоляции ReadCommitted
	tx, err := m.pool.BeginTx(
		txCtx,
		pgx.TxOptions{
			IsoLevel: pgx.ReadCommitted,
		},
	)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	// Сохраняем транзакцию в context
	txCtx = context.WithValue(txCtx, txContextKey{}, tx)

	committed := false

	// Если транзакция не подтверждена, откатываем её
	defer func() {
		if committed {
			return
		}

		rollbackCtx, rollbackCancel := context.WithTimeout(
			context.WithoutCancel(ctx),
			m.timeout,
		)
		defer rollbackCancel()

		rollbackErr := tx.Rollback(rollbackCtx)

		if rollbackErr != nil &&
			!errors.Is(rollbackErr, pgx.ErrTxClosed) {
			err = errors.Join(
				err,
				fmt.Errorf("rollback transaction: %w", rollbackErr),
			)
		}
	}()

	// Выполняем переданную бизнес-операцию
	if err = fn(txCtx); err != nil {
		return fmt.Errorf("transaction operation: %w", err)
	}

	// Подтверждаем изменения
	if err = tx.Commit(txCtx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	committed = true

	return nil
}
