package ranking

import (
	"context"
	"gorm.io/gorm"
	"tb_live_module/global"
	"tb_live_module/internal/modulekit"
	"time"
	_ "time/tzdata"
)

type Service struct {
	DB       *gorm.DB
	Now      func() time.Time
	Timezone string
}

var Default = &Service{Timezone: "Asia/Kuala_Lumpur"}

type ServiceGroup struct{ *Service }

func NewServiceGroup() ServiceGroup { return ServiceGroup{Default} }

type Actor = modulekit.Actor

var ErrInvalid = modulekit.Invalid
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
func (s *Service) zone() string {
	if s.Timezone == "" {
		return "Asia/Kuala_Lumpur"
	}
	return s.Timezone
}
func (s *Service) transaction(ctx context.Context, fn func(*gorm.DB) error) error {
	return modulekit.Transaction(s.db(ctx), ctx, fn)
}
