// Package social 封装数据库锁、重试和计数不变量，不依赖 live 业务表。
package social

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"math/rand/v2"
	"sort"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	model "tb_live_module/model/social"
)

var ErrCounterInvariant = errors.New("social counter invariant violated")

const MaxSafeInteger uint64 = 9007199254740991

type Repository struct {
	DB      *gorm.DB
	OnRetry func(uint16)
}

func (r Repository) Transaction(ctx context.Context, fn func(*gorm.DB) error) error {
	if r.DB == nil {
		return errors.New("social database unavailable")
	}
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		err = r.DB.WithContext(ctx).Session(&gorm.Session{NowFunc: func() time.Time { return time.Now().UTC() }}).Transaction(fn, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		var me *mysql.MySQLError
		if !errors.As(err, &me) || (me.Number != 1213 && me.Number != 1205) || attempt == 3 {
			return err
		}
		if r.OnRetry != nil {
			r.OnRetry(me.Number)
		}
		timer := time.NewTimer(time.Duration((1<<attempt)*10+rand.IntN(15)) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}

// LockUsers 所有关系变更先锁同一批 counter 行。逐主键升序获取，不能先锁关系。
func LockUsers(tx *gorm.DB, ids ...uint64) (map[uint64]model.SocialUserCounter, error) {
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	rows := make(map[uint64]model.SocialUserCounter, len(ids))
	for _, id := range ids {
		if id == 0 || id > MaxSafeInteger {
			return nil, ErrCounterInvariant
		}
		if _, ok := rows[id]; ok {
			continue
		}
		var row model.SocialUserCounter
		locked := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", id).Limit(1).Find(&row)
		if locked.Error != nil {
			return nil, locked.Error
		}
		if locked.RowsAffected == 0 {
			// 只有缺失行才执行 upsert，已存在的热门计数行省去一次写操作。
			row.UserID = id
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return nil, err
			}
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "user_id = ?", id).Error; err != nil {
				return nil, err
			}
		}
		rows[id] = row
	}
	return rows, nil
}
func BumpVersions(tx *gorm.DB, ids ...uint64) error {
	r := tx.Model(&model.SocialUserCounter{}).Where("user_id IN ? AND version < ?", ids, MaxSafeInteger).Update("version", gorm.Expr("version + 1"))
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != int64(len(ids)) {
		return ErrCounterInvariant
	}
	return nil
}
func CountDelta(tx *gorm.DB, id uint64, column string, delta int) error {
	if column != "following_count" && column != "fans_count" {
		return ErrCounterInvariant
	}
	q := tx.Model(&model.SocialUserCounter{}).Where("user_id = ?", id)
	if delta == 1 {
		q = q.Where(column+" < ?", uint64(math.MaxInt64))
	} else if delta == -1 {
		q = q.Where(column + " > 0")
	} else {
		return ErrCounterInvariant
	}
	r := q.Update(column, gorm.Expr(column+" + ?", delta))
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return ErrCounterInvariant
	}
	return nil
}
func Blocked(tx *gorm.DB, a, b uint64) (bool, error) {
	var n int64
	err := tx.Model(&model.SocialBlock{}).Where("(user_id = ? AND target_user_id = ?) OR (user_id = ? AND target_user_id = ?)", a, b, b, a).Count(&n).Error
	return n > 0, err
}
func Following(tx *gorm.DB, a, b uint64) (bool, error) {
	var n int64
	err := tx.Model(&model.SocialFollow{}).Where("user_id = ? AND target_user_id = ?", a, b).Count(&n).Error
	return n > 0, err
}

// InsertAbsent 必须在LockUsers之后调用。避免依赖MySQL clientFoundRows参数推断幂等结果。
func InsertAbsent(tx *gorm.DB, row interface{}, predicate string, args ...interface{}) (bool, error) {
	var n int64
	if err := tx.Model(row).Where(predicate, args...).Count(&n).Error; err != nil {
		return false, err
	}
	if n > 0 {
		return false, nil
	}
	if err := tx.Create(row).Error; err != nil {
		return false, err
	}
	return true, nil
}
