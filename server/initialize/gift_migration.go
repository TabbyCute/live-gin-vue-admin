package initialize

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// MySQL命名锁属于连接，必须用同一个专用连接完成获取、DDL和释放。
// 仅串行礼物迁移/菜单同步，业务请求和其他数据库不受影响；不增加锁表。
func withGiftMigrationLock(db *gorm.DB, fn func(*gorm.DB) error) error {
	return withFinancialMigrationLock(db, 5*time.Minute, fn)
}

// Administrative migrations may explicitly budget for remote catalog/DDL
// round trips; ordinary startup retains the original five-minute limit.
func withFinancialMigrationLock(db *gorm.DB, budget time.Duration, fn func(*gorm.DB) error) error {
	if db == nil {
		return fmt.Errorf("gift requires database")
	}
	if db.Dialector.Name() != "mysql" {
		return fn(db)
	}
	ctx, cancel := context.WithTimeout(db.Statement.Context, budget)
	defer cancel()
	return db.WithContext(ctx).Connection(func(conn *gorm.DB) (err error) {
		// Connection回调给出可变Statement；从干净会话派生每条SQL，避免前一条
		// Raw/Migrator残留的表名、错误或条件污染下一条命令。物理连接仍保持不变。
		conn = conn.Session(&gorm.Session{NewDB: true})
		var database string
		if err = conn.Raw("SELECT DATABASE()").Scan(&database).Error; err != nil {
			return fmt.Errorf("gift migration database: %w", err)
		}
		if database == "" {
			return fmt.Errorf("gift migration requires selected database")
		}
		digest := sha256.Sum256([]byte(database))
		name := fmt.Sprintf("gift:migrate:%x", digest[:24])
		// 即使GET_LOCK响应丢失/取消，也尝试释放可能已获得的锁。
		defer func() {
			cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			var released sql.NullInt64
			if releaseErr := conn.WithContext(cleanup).Raw("SELECT RELEASE_LOCK(?)", name).Scan(&released).Error; releaseErr != nil {
				// 不能把可能仍持锁的连接交回池；Raw返回ErrBadConn会淘汰该物理连接。
				if sqlConn, ok := conn.Statement.ConnPool.(*sql.Conn); ok {
					_ = sqlConn.Raw(func(any) error { return driver.ErrBadConn })
				}
				err = errors.Join(err, fmt.Errorf("release gift migration lock: %w", releaseErr))
			}
		}()
		var acquired sql.NullInt64
		if err = conn.Raw("SELECT GET_LOCK(?, 30)", name).Scan(&acquired).Error; err != nil {
			return fmt.Errorf("acquire gift migration lock: %w", err)
		}
		if !acquired.Valid || acquired.Int64 != 1 {
			return fmt.Errorf("gift migration is busy; retry after the other migration completes")
		}
		return fn(conn)
	})
}
