package guard

import (
	"context"
	"gorm.io/gorm"
	"tb_live_module/global"
	"tb_live_module/internal/modulekit"
	"time"
)

type Service struct {
	DB  *gorm.DB
	Now func() time.Time
}

var Default = &Service{}

type ServiceGroup struct{ *Service }

func NewServiceGroup() ServiceGroup { return ServiceGroup{Default} }

type Actor = modulekit.Actor

var ErrInvalid = modulekit.Invalid
var ErrConflict = modulekit.Conflict
var ErrInvariant = modulekit.Invariant

func (s *Service) db(ctx context.Context) *gorm.DB {
	d := s.DB
	if d == nil {
		d = global.GVA_DB
	}
	return d.WithContext(ctx)
}
func (s *Service) now() int64 {
	if s.Now != nil {
		return s.Now().UnixMilli()
	}
	return time.Now().UnixMilli()
}
func (s *Service) transaction(ctx context.Context, fn func(*gorm.DB) error) error {
	return modulekit.Transaction(s.db(ctx), ctx, fn)
}

const DayMillis int64 = 86400000
const MaxDays int64 = 36500000 // 3650 days x 10000 gifts, overflow checked separately.
const maxFacts = 10000
