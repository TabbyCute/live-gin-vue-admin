package bankreconcile

import (
	"context"
	"gorm.io/gorm"
	"tb_live_module/global"
	payout "tb_live_module/service/payout"
)

type Service struct {
	DB     *gorm.DB
	Payout *payout.Service
}

var Default = &Service{}

type ServiceGroup struct{ *Service }

func NewServiceGroup() ServiceGroup { return ServiceGroup{Default} }
func (s *Service) db(ctx context.Context) *gorm.DB {
	db := s.DB
	if db == nil {
		db = global.GVA_DB
	}
	return db.WithContext(ctx)
}
func (s *Service) payout() *payout.Service {
	if s.Payout != nil {
		return s.Payout
	}
	return payout.Default
}
