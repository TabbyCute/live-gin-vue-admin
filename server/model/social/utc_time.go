package social

import (
	"database/sql/driver"
	"fmt"
	"time"
)

// UTCTime 保留项目created_at/updated_at的datetime(3)结构，但仅为social表固定UTC语义。
// 使用字符串绑定避免mysql驱动loc=Local将time.Time转换为本地墙钟；读取时重建UTC墙钟，
// 不修改其他模块历史日期，也不要求全库切换DSN时区。业务DTO仍只输出Unix毫秒。
type UTCTime struct{ time.Time }

func NewUTCTime(t time.Time) UTCTime { return UTCTime{Time: t.UTC()} }
func (UTCTime) GormDataType() string { return "time" }
func (t UTCTime) Value() (driver.Value, error) {
	if t.IsZero() {
		return nil, nil
	}
	return t.UTC().Format("2006-01-02 15:04:05.000"), nil
}
func (t *UTCTime) Scan(value interface{}) error {
	switch v := value.(type) {
	case nil:
		t.Time = time.Time{}
		return nil
	case time.Time:
		// MySQL parseTime会附加连接loc，但DATETIME自身没有时区；这里保留数据库数字。
		t.Time = time.Date(v.Year(), v.Month(), v.Day(), v.Hour(), v.Minute(), v.Second(), v.Nanosecond(), time.UTC)
		return nil
	case []byte:
		return t.Scan(string(v))
	case string:
		for _, layout := range []string{"2006-01-02 15:04:05.999999999", time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00"} {
			parsed, err := time.ParseInLocation(layout, v, time.UTC)
			if err == nil {
				t.Time = parsed.UTC()
				return nil
			}
		}
	}
	return fmt.Errorf("invalid social UTC datetime type %T", value)
}
