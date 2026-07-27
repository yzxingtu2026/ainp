package outbox

import (
	"context"
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/gorm"
)

const notificationChannel = "ainp_outbox_wakeup"

// CommitNotifier emits a transaction-bound hint after an Event is inserted.
// PostgreSQL delivers pg_notify only after the surrounding transaction commits.
type CommitNotifier interface {
	Notify(context.Context, *gorm.DB) error
}

type postgresCommitNotifier struct{}

func NewPostgresCommitNotifier() CommitNotifier { return postgresCommitNotifier{} }

func (postgresCommitNotifier) Notify(ctx context.Context, tx *gorm.DB) error {
	if tx == nil {
		return fmt.Errorf("outbox: notify transaction is required")
	}
	if err := tx.WithContext(ctx).Exec("SELECT pg_notify(?, '')", notificationChannel).Error; err != nil {
		return fmt.Errorf("outbox: notify commit wakeup: %w", err)
	}
	return nil
}

// NotificationListener blocks on a dedicated connection and emits wake hints.
type NotificationListener interface {
	Listen(context.Context, func()) error
}

type postgresNotificationListener struct{ db *gorm.DB }

func NewPostgresNotificationListener(db *gorm.DB) NotificationListener {
	return &postgresNotificationListener{db: db}
}

func (l *postgresNotificationListener) Listen(ctx context.Context, wake func()) error {
	if l == nil || l.db == nil {
		return fmt.Errorf("outbox: listener database is required")
	}
	sqlDB, err := l.db.DB()
	if err != nil {
		return fmt.Errorf("outbox: listener database handle: %w", err)
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("outbox: listener connection: %w", err)
	}
	defer conn.Close()
	return conn.Raw(func(raw any) error {
		stdlibConn, ok := raw.(*stdlib.Conn)
		if !ok {
			return fmt.Errorf("outbox: PostgreSQL pgx connection is required, got %T", raw)
		}
		pgxConn := stdlibConn.Conn()
		if _, err := pgxConn.Exec(ctx, "LISTEN "+notificationChannel); err != nil {
			return fmt.Errorf("outbox: listen: %w", err)
		}
		defer func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, _ = pgxConn.Exec(cleanupCtx, "UNLISTEN "+notificationChannel)
		}()
		// Scan once after LISTEN so events committed during connection setup are
		// covered even though PostgreSQL does not replay earlier notifications.
		wake()
		for {
			if _, err := pgxConn.WaitForNotification(ctx); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if err == driver.ErrBadConn {
					return err
				}
				return fmt.Errorf("outbox: wait notification: %w", err)
			}
			wake()
		}
	})
}
