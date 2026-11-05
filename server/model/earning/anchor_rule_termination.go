package earning

// RuleTermination records a separate, two-person decision; original rule windows remain immutable.
type RuleTermination struct {
	ID            uint64 `json:"-" gorm:"primaryKey"`
	TerminationNo string `json:"terminationNo" gorm:"size:40;not null;uniqueIndex"`
	RequestID     string `json:"requestId" gorm:"size:64;not null;uniqueIndex"`
	RuleNo        string `json:"ruleNo" gorm:"size:40;not null;index:idx_rule_termination,priority:1"`
	Reason        string `json:"reason" gorm:"size:500;not null"`
	Status        string `json:"status" gorm:"size:16;not null;index:idx_rule_termination,priority:2;check:ck_rule_termination_status,status IN ('pending','approved','rejected')"`
	CreatedBy     uint64 `json:"createdBy,string" gorm:"not null"`
	ReviewedBy    uint64 `json:"reviewedBy,string" gorm:"not null;default:0"`
	CreatedAt     int64  `json:"createdAt" gorm:"not null"`
	ReviewedAt    int64  `json:"reviewedAt" gorm:"not null;default:0"`
}

func (RuleTermination) TableName() string { return "anchor_rule_termination" }
