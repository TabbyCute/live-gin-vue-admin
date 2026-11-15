package internal

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"strings"
	"tb_live_module/global"
	"tb_live_module/model/system"
	"tb_live_module/service"
	"time"
)

// Core is immutable after construction. Business labels stay structured fields,
// never a dynamic directory or a concurrent mutation of a shared logging Core.
type ZapCore struct {
	level      zapcore.Level
	boundTrace string
	zapcore.Core
}

var errorPersistenceSlots = make(chan struct{}, 2)

func NewZapCore(level zapcore.Level) *ZapCore {
	z := &ZapCore{level: level}
	z.Core = zapcore.NewCore(global.GVA_CONFIG.Zap.Encoder(), z.WriteSyncer(), zap.LevelEnablerFunc(func(l zapcore.Level) bool { return l == level }))
	return z
}
func (z *ZapCore) WriteSyncer(formats ...string) zapcore.WriteSyncer {
	c := NewCutter(global.GVA_CONFIG.Zap.Director, z.level.String(), global.GVA_CONFIG.Zap.RetentionDay, CutterWithLayout(time.DateOnly), CutterWithFormats(formats...))
	if global.GVA_CONFIG.Zap.LogInConsole {
		return zapcore.NewMultiWriteSyncer(os.Stdout, c)
	}
	return c
}
func (z *ZapCore) Enabled(level zapcore.Level) bool { return z.level == level }
func (z *ZapCore) With(fields []zapcore.Field) zapcore.Core {
	return &ZapCore{level: z.level, boundTrace: traceFromFields(z.boundTrace, fields), Core: z.Core.With(fields)}
}
func (z *ZapCore) Check(entry zapcore.Entry, check *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if z.Enabled(entry.Level) {
		return check.AddCore(entry, z)
	}
	return check
}
func (z *ZapCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	err := z.Core.Write(entry, fields)
	if entry.Level < zapcore.ErrorLevel || global.GVA_DB == nil || strings.Contains(entry.Caller.File, "gorm_logger_writer.go") {
		return err
	}
	select {
	case errorPersistenceSlots <- struct{}{}:
		defer func() { <-errorPersistenceSlots }()
	default:
		return err
	}
	// Database records omit free-form errors, source code and request data. The
	// trace links the structured log without persisting another sensitive copy.
	form, level := "后端", entry.Level.String()
	info := fmt.Sprintf("level=%s caller=%s:%d", level, entry.Caller.File, entry.Caller.Line)
	if trace := traceFromFields(z.boundTrace, fields); trace != "" {
		info += " traceId=" + trace
	}
	persistSystemError(&system.SysError{Form: &form, Info: &info, Level: level})
	return err
}

func traceFromFields(previous string, fields []zapcore.Field) string {
	for _, f := range fields {
		if (f.Key == "traceId" || f.Key == "trace") && f.Type == zapcore.StringType && len(f.String) == 39 && strings.HasPrefix(f.String, "rt_") {
			if _, e := uuid.Parse(f.String[3:]); e == nil {
				previous = f.String
			}
		}
	}
	return previous
}
func persistSystemError(record *system.SysError) {
	defer func() { _ = recover() }()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_ = service.ServiceGroupApp.SystemServiceGroup.SysErrorService.CreateSysError(ctx, record)
}
func (z *ZapCore) Sync() error { return z.Core.Sync() }
