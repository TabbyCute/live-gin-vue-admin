package earning

// AuditJob is an operational report, never a financial authorization or a
// checkpoint accepted by settlement, withdrawal, or payout services.
type AuditJob struct {
	ID              uint64 `json:"-" gorm:"primaryKey"`
	JobNo           string `json:"jobNo" gorm:"size:40;not null;uniqueIndex"`
	RequestID       string `json:"requestId" gorm:"size:64;not null;uniqueIndex"`
	RequestHash     string `json:"-" gorm:"size:64;not null"`
	AnchorID        uint64 `json:"-" gorm:"not null;index:idx_earning_audit_anchor,priority:1"`
	AnchorNo        string `json:"anchorNo" gorm:"size:40;not null;index"`
	Currency        string `json:"currency" gorm:"size:16;not null"`
	RetryOf         string `json:"retryOf" gorm:"size:40;not null"`
	Status          string `json:"status" gorm:"size:24;not null;index:idx_earning_audit_queue,priority:1;index:idx_earning_audit_anchor,priority:2;check:ck_earning_audit_status,status IN ('queued','running','cancel_requested','cancelled','passed','failed')"`
	Progress        string `json:"progress" gorm:"type:text;not null"`
	AccountSnapshot string `json:"accountSnapshot" gorm:"type:text;not null"`
	SnapshotHash    string `json:"snapshotHash" gorm:"size:64;not null"`
	IssueCode       string `json:"issueCode" gorm:"size:40;not null"`
	CreatedBy       uint64 `json:"createdBy,string" gorm:"not null"`
	CancelledBy     uint64 `json:"cancelledBy,string" gorm:"not null"`
	CreatedAt       int64  `json:"createdAt" gorm:"not null;index:idx_earning_audit_queue,priority:2"`
	StartedAt       int64  `json:"startedAt" gorm:"not null"`
	UpdatedAt       int64  `json:"updatedAt" gorm:"not null"`
	FinishedAt      int64  `json:"finishedAt" gorm:"not null"`
}

func (AuditJob) TableName() string { return "anchor_earning_audit_job" }
