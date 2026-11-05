package payout

import (
	"gorm.io/gorm"
	pm "tb_live_module/model/payout"
)

// Key retirement must include every company account and evidence object, even
// disabled accounts and terminal attempts. Never log IDs together with secrets.
func (s *Service) ValidateManualKeys(db *gorm.DB) error {
	if s == nil || s.Crypto == nil {
		return ErrCrypto
	}
	for _, m := range []any{&pm.ChannelAccount{}, &pm.Evidence{}} {
		var ids []string
		if e := db.Model(m).Distinct("encryption_key_id").Limit(17).Pluck("encryption_key_id", &ids).Error; e != nil {
			return e
		}
		if len(ids) > 16 {
			return ErrCrypto
		}
		for _, id := range ids {
			if _, ok := s.Crypto.keys[id]; !ok {
				return ErrCrypto
			}
			switch m.(type) {
			case *pm.ChannelAccount:
				var c pm.ChannelAccount
				if e := db.Where("encryption_key_id=?", id).Order("id ASC").Take(&c).Error; e != nil {
					return e
				}
				if _, e := s.openCompany(c); e != nil {
					return e
				}
			case *pm.Evidence:
				var ev pm.Evidence
				if e := db.Where("encryption_key_id=?", id).Order("evidence_no ASC").Take(&ev).Error; e != nil {
					return e
				}
				b, e := s.readEvidenceBytes(db.Statement.Context, ev)
				clear(b)
				if e != nil {
					return e
				}
			}
		}
	}
	return nil
}
