package eventdelivery

import (
	"context"
	"database/sql"
	"errors"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"time"
)

// Only local database work may use this retry helper; never replay an adapter call.
func (s *Service) transaction(ctx context.Context, fn func(*gorm.DB) error) error {
	for attempt := 0; ; attempt++ {
		err := s.DB.WithContext(ctx).Transaction(fn, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		var mysqlErr *driver.MySQLError
		if attempt >= 3 || !errors.As(err, &mysqlErr) || (mysqlErr.Number != 1213 && mysqlErr.Number != 1205) {
			return err
		}
		timer := time.NewTimer(time.Duration(10*(1<<attempt)) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
