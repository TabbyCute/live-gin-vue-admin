package earning

// RevenueRule 不使用软删除；财务事实由数据库触发器保护。
type RevenueRule struct {
	ID             uint64 `json:"-" gorm:"primaryKey"`
	RuleNo         string `json:"ruleNo" gorm:"size:40;not null;uniqueIndex"`
	Name           string `json:"name" gorm:"size:64;not null"`
	ScopeType      string `json:"scopeType" gorm:"size:16;not null;index:idx_revenue_scope,priority:1"`
	ScopeNo        string `json:"scopeNo" gorm:"size:40;not null;index:idx_revenue_scope,priority:2"`
	Priority       int32  `json:"priority" gorm:"not null"`
	AnchorShareBPS int64  `json:"anchorShareBps,string" gorm:"not null;check:ck_revenue_rule_share,anchor_share_bps BETWEEN 0 AND 10000"`
	EffectiveFrom  int64  `json:"effectiveFrom" gorm:"not null"`
	EffectiveTo    int64  `json:"effectiveTo" gorm:"not null;check:ck_revenue_rule_window,effective_to > effective_from"`
	Status         string `json:"status" gorm:"size:16;not null;check:ck_revenue_rule_status,status IN ('pending','active','rejected')"`
	Version        int64  `json:"version,string" gorm:"not null"`
	CreatedBy      uint64 `json:"createdBy,string" gorm:"not null"`
	ApprovedBy     uint64 `json:"approvedBy,string" gorm:"not null"`
	CreatedAt      int64  `json:"createdAt" gorm:"not null"`
	ApprovedAt     int64  `json:"approvedAt" gorm:"not null"`
}

func (RevenueRule) TableName() string { return "anchor_revenue_rule" }
