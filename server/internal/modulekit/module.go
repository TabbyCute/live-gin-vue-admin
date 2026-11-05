// Package modulekit contains only shared mechanics, never business balances or facts.
package modulekit

import (
	"context"
	"database/sql"
	"errors"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"regexp"
	"strings"
	"tb_live_module/model/system"
	"time"
)

type Error struct {
	Code    int
	Message string
}

func (e *Error) Error() string { return e.Message }

var Invalid = &Error{49001, "参数无效"}
var Forbidden = &Error{49002, "无操作权限"}
var Conflict = &Error{49003, "请求参数或版本冲突，请刷新重试"}
var Unavailable = &Error{49004, "服务暂不可用，请稍后重试"}
var Invariant = &Error{49005, "数据核验异常，已停止处理"}
var NotFound = &Error{49006, "记录不存在"}
var Number = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)
var RequestID = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

type Actor struct {
	ID          uint64
	AuthorityID uint
	IP          string
}

func No(prefix string) string { return prefix + strings.ReplaceAll(uuid.NewString(), "-", "") }
func Authorize(tx *gorm.DB, a Actor, table, path, method string) error {
	if a.ID == 0 {
		return Forbidden
	}
	var u system.SysUser
	if e := tx.Where("id=? AND enable=1", a.ID).Take(&u).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return Forbidden
		}
		return e
	}
	if system.IsSuperAdmin(a.ID) {
		return nil
	}
	var n int64
	if e := tx.Table("sys_user_authority AS m").Joins("JOIN sys_authorities AS r ON r.authority_id=m.sys_authority_authority_id AND r.deleted_at IS NULL").Where("m.sys_user_id=? AND m.sys_authority_authority_id=?", a.ID, a.AuthorityID).Count(&n).Error; e != nil {
		return e
	}
	if n == 0 {
		return Forbidden
	}
	if e := tx.Table(table).Where("authority_id=? AND path=? AND method=?", a.AuthorityID, path, method).Count(&n).Error; e != nil {
		return e
	}
	if n == 0 {
		return Forbidden
	}
	return nil
}
func Transaction(db *gorm.DB, ctx context.Context, fn func(*gorm.DB) error) error {
	for i := 0; ; i++ {
		e := db.WithContext(ctx).Transaction(fn, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		var me *driver.MySQLError
		if i >= 3 || !errors.As(e, &me) || (me.Number != 1205 && me.Number != 1213) {
			return e
		}
		t := time.NewTimer(time.Duration(10<<i) * time.Millisecond)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}
