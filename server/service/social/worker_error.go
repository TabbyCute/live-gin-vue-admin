package social

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// 不把驱动原始错误写入日志：它可能包含SQL参数、缓存内容或凭证。
type workerStepError struct {
	stage string
	shard int
	err   error
}

func (e *workerStepError) Error() string {
	return fmt.Sprintf("%s failed (shard %d)", e.stage, e.shard)
}
func (e *workerStepError) Unwrap() error { return e.err }

func workerErrorFields(err error) []zap.Field {
	fields := []zap.Field{zap.String("error_type", fmt.Sprintf("%T", err))}
	var step *workerStepError
	if errors.As(err, &step) {
		fields = append(fields, zap.String("stage", step.stage), zap.Int("shard", step.shard))
	}
	kind := "internal"
	var db *mysql.MySQLError
	var re redis.Error
	var ne net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		kind = "deadline_exceeded"
	case errors.Is(err, context.Canceled):
		kind = "canceled"
	case errors.As(err, &db):
		kind = "mysql"
		fields = append(fields, zap.Uint16("mysql_code", db.Number))
	case errors.As(err, &re):
		kind = "redis"
		code, _, _ := strings.Cut(re.Error(), " ")
		// Redis错误首词也只接受有限标准码，不记录任意服务端内容。
		switch code {
		case "WRONGTYPE", "NOAUTH", "NOPERM", "READONLY", "OOM", "LOADING", "CLUSTERDOWN", "CROSSSLOT", "NOSCRIPT", "BUSY", "ERR":
			fields = append(fields, zap.String("redis_code", code))
		}
	case errors.As(err, &ne):
		kind = "network"
		if ne.Timeout() {
			kind = "network_timeout"
		}
	}
	return append(fields, zap.String("error_kind", kind))
}
