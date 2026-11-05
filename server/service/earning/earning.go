package earning

import (
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	model "tb_live_module/model/earning"
)

var ErrNoRule = errors.New("没有当前有效分成规则，禁止送礼")
var ErrRuleConflict = errors.New("分成规则冲突或不能自审")

// Share avoids intermediate multiplication overflow and rounds down to whole COIN.
func Share(amount, bps int64) int64 { return amount/10000*bps + amount%10000*bps/10000 }

// SelectInTx requires the caller to hold gift_control SHARE until commit.
// Fixed precedence: anchor > gift > category > global; then priority DESC, version DESC, ID DESC.
func (Service) SelectInTx(tx *gorm.DB, anchor, gift, category string, now int64) (model.RevenueRule, error) {
	var r model.RevenueRule
	e := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("status = 'active' AND effective_from <= ? AND effective_to > ?", now, now).
		Where("NOT EXISTS (SELECT 1 FROM anchor_rule_termination t WHERE t.rule_no=anchor_revenue_rule.rule_no AND t.status='approved' AND t.reviewed_at <= ?)", now).
		Where("(scope_type='anchor' AND scope_no=?) OR (scope_type='gift' AND scope_no=?) OR (scope_type='category' AND scope_no=?) OR (scope_type='global' AND scope_no='')", anchor, gift, category).
		Order("CASE scope_type WHEN 'anchor' THEN 4 WHEN 'gift' THEN 3 WHEN 'category' THEN 2 ELSE 1 END DESC, priority DESC, version DESC, id DESC").Take(&r).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		e = ErrNoRule
	}
	return r, e
}
func (Service) AppendInTx(tx *gorm.DB, anchor uint64, order, business, rule string, bps, amount, gross, now int64, reversal bool) (string, error) {
	no := "ge_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	kind := "earning"
	if reversal {
		kind = "reversal"
	}
	row := model.Ledger{EarningNo: no, AnchorID: anchor, GiftOrderNo: order, BusinessNo: business, EntryType: kind, RevenueRuleNo: rule, ShareBPS: bps, EarningCoinAmount: amount, Status: "pending", CreatedAt: now}
	if e := tx.Create(&row).Error; e != nil {
		return "", e
	}
	_, e := RecordGiftIncomeInTx(tx, row, gross)
	return no, e
}
