package guard

import (
	"context"
	"database/sql"
	"encoding/json"
	"gorm.io/gorm"
	model "tb_live_module/model/guard"
)

// VerifyAll is read-only, bounded by the caller's deadline and batches. It is
// required before enabling new sales; no corrupt projection is silently repaired.
func (s *Service) VerifyAll(ctx context.Context) error {
	return s.db(ctx).Transaction(func(tx *gorm.DB) error {
		var after uint64
		for {
			var members []model.Membership
			if e := tx.Where("id>?", after).Order("id").Limit(100).Find(&members).Error; e != nil {
				return e
			}
			if len(members) == 0 {
				return nil
			}
			for _, m := range members {
				after = m.ID
				g, c, e := loadFacts(tx, m)
				if e != nil {
					return e
				}
				segments, e := Replay(g, c)
				if e != nil {
					return e
				}
				raw, _ := json.Marshal(segments)
				if m.Projection != string(raw) {
					return ErrInvariant
				}
				var l model.Ledger
				if e = tx.Where("ledger_no=? AND membership_no=?", m.LedgerNo, m.MembershipNo).Take(&l).Error; e != nil {
					return e
				}
				if l.AfterSnapshot != m.Projection {
					return ErrInvariant
				}
			}
		}
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
}
