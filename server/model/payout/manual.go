package payout

// Storage models are deliberately not HTTP response types.
type Channel struct {
	ChannelCode string `json:"-" gorm:"size:32;primaryKey"`
	ChannelType string `json:"-" gorm:"size:16;not null;check:ck_payout_channel,channel_code='manual_bank' AND channel_type='manual'"`
	CreatedAt   int64  `json:"-" gorm:"not null"`
}

func (Channel) TableName() string { return "payout_channel" }

type ChannelAccount struct {
	ID                 uint64 `json:"-" gorm:"primaryKey"`
	ChannelAccountNo   string `json:"-" gorm:"size:40;not null;uniqueIndex"`
	ChannelCode        string `json:"-" gorm:"size:32;not null;index;uniqueIndex:uk_company_fingerprint,priority:1"`
	AccountName        string `json:"-" gorm:"size:128;not null"`
	InstitutionName    string `json:"-" gorm:"size:128;not null"`
	Currency           string `json:"-" gorm:"size:8;not null;index;uniqueIndex:uk_company_fingerprint,priority:2"`
	EncryptedAccount   string `json:"-" gorm:"type:text;not null"`
	EncryptionKeyID    string `json:"-" gorm:"size:32;not null;index"`
	AccountFingerprint string `json:"-" gorm:"size:64;not null;uniqueIndex:uk_company_fingerprint,priority:3"`
	MaskedAccount      string `json:"-" gorm:"size:128;not null"`
	Status             string `json:"-" gorm:"size:16;not null;index;check:ck_company_status,status IN ('active','disabled')"`
	SingleLimit        int64  `json:"-" gorm:"not null;check:ck_company_limits,single_limit>0 AND daily_limit>=single_limit"`
	DailyLimit         int64  `json:"-" gorm:"not null"`
	CreatedBy          uint64 `json:"-" gorm:"not null"`
	CreatedAt          int64  `json:"-" gorm:"not null"`
	UpdatedAt          int64  `json:"-" gorm:"not null;autoUpdateTime:false"`
	DisabledAt         int64  `json:"-" gorm:"not null"`
	Version            int64  `json:"-" gorm:"not null;check:ck_company_version,version>0"`
	LastAuditNo        string `json:"-" gorm:"size:40;not null"`
}

func (ChannelAccount) TableName() string { return "payout_channel_account" }

type Attempt struct {
	LimitDay                    string  `json:"-" gorm:"size:10;not null;uniqueIndex:uk_attempt_usage,priority:2"`
	UsageVersion                int64   `json:"-" gorm:"not null;uniqueIndex:uk_attempt_usage,priority:3"`
	ID                          uint64  `json:"-" gorm:"primaryKey"`
	AttemptNo                   string  `json:"-" gorm:"size:40;not null;uniqueIndex"`
	PayoutNo                    string  `json:"-" gorm:"size:40;not null;uniqueIndex:uk_attempt_sequence,priority:1;uniqueIndex:uk_attempt_active,priority:1;uniqueIndex:uk_attempt_success,priority:1"`
	Sequence                    int64   `json:"-" gorm:"not null;uniqueIndex:uk_attempt_sequence,priority:2;check:ck_attempt_sequence,sequence>0 AND version>0 AND usage_version>0"`
	Version                     int64   `json:"-" gorm:"not null"`
	ActiveSlot                  *int    `json:"-" gorm:"uniqueIndex:uk_attempt_active,priority:2;check:ck_attempt_active,((status IN ('processing','submitted','unknown')) AND active_slot IS NOT NULL AND active_slot=1) OR ((status IN ('succeeded','failed','abandoned')) AND active_slot IS NULL)"`
	SuccessSlot                 *int    `json:"-" gorm:"uniqueIndex:uk_attempt_success,priority:2;check:ck_attempt_success,(status='succeeded' AND success_slot IS NOT NULL AND success_slot=1) OR (status<>'succeeded' AND success_slot IS NULL)"`
	ChannelCode                 string  `json:"-" gorm:"size:32;not null"`
	ChannelAccountNo            string  `json:"-" gorm:"size:40;not null;uniqueIndex:uk_payment_reference,priority:1;uniqueIndex:uk_attempt_usage,priority:1;index:idx_attempt_company_time,priority:1"`
	CompanyMasked               string  `json:"-" gorm:"size:128;not null"`
	Amount                      int64   `json:"-" gorm:"not null;check:ck_attempt_amount,amount>0"`
	Currency                    string  `json:"-" gorm:"size:8;not null"`
	PayeeSnapshotHash           string  `json:"-" gorm:"size:64;not null"`
	Status                      string  `json:"-" gorm:"size:16;not null;index;check:ck_attempt_status,status IN ('processing','submitted','succeeded','failed','unknown','abandoned')"`
	OperatorID                  uint64  `json:"-" gorm:"not null;index"`
	ReviewerID                  uint64  `json:"-" gorm:"not null;index"`
	PaymentReference            string  `json:"-" gorm:"size:128;not null"`
	PaymentReferenceFingerprint *string `json:"-" gorm:"size:64;uniqueIndex:uk_payment_reference,priority:2"`
	PaymentTime                 int64   `json:"-" gorm:"not null"`
	EvidenceReference           string  `json:"-" gorm:"size:40;not null;index"`
	EvidenceHash                string  `json:"-" gorm:"size:64;not null"`
	OperatorNote                string  `json:"-" gorm:"size:500;not null"`
	ReviewReason                string  `json:"-" gorm:"size:500;not null"`
	BankSubmittedAt             int64   `json:"-" gorm:"not null"`
	SubmittedAt                 int64   `json:"-" gorm:"not null"`
	ReviewedAt                  int64   `json:"-" gorm:"not null"`
	CompletedAt                 int64   `json:"-" gorm:"not null"`
	FailureCode                 string  `json:"-" gorm:"size:40;not null"`
	FailureReason               string  `json:"-" gorm:"size:500;not null"`
	CreatedAt                   int64   `json:"-" gorm:"not null;index:idx_attempt_company_time,priority:2"`
	UpdatedAt                   int64   `json:"-" gorm:"not null;autoUpdateTime:false"`
	LastEventNo                 string  `json:"-" gorm:"size:40;not null"`
}

func (Attempt) TableName() string { return "payout_attempt" }

// Event snapshots authenticate each permitted change against immutable evidence.
type AttemptEvent struct {
	ID           uint64 `json:"-" gorm:"primaryKey"`
	EventNo      string `json:"eventNo" gorm:"size:40;not null;uniqueIndex"`
	AttemptNo    string `json:"attemptNo" gorm:"size:40;not null;uniqueIndex:uk_attempt_event_version,priority:1"`
	Version      int64  `json:"version,string" gorm:"not null;uniqueIndex:uk_attempt_event_version,priority:2"`
	PayoutNo     string `json:"payoutNo" gorm:"size:40;not null;index"`
	Action       string `json:"action" gorm:"size:40;not null"`
	FromStatus   string `json:"fromStatus" gorm:"size:16;not null"`
	ToStatus     string `json:"toStatus" gorm:"size:16;not null"`
	ActorID      uint64 `json:"actorId,string" gorm:"not null"`
	Reason       string `json:"reason" gorm:"size:500;not null"`
	TransitionNo string `json:"transitionNo,omitempty" gorm:"size:40;not null"`
	Snapshot     string `json:"-" gorm:"type:text;not null"`
	SnapshotHash string `json:"snapshotHash" gorm:"size:64;not null"`
	CreatedAt    int64  `json:"createdAt" gorm:"not null"`
}

func (AttemptEvent) TableName() string { return "payout_attempt_event" }

type ManualRequest struct {
	RequestID   string `json:"-" gorm:"size:64;primaryKey"`
	RequestHash string `json:"-" gorm:"size:64;not null"`
	ActorID     uint64 `json:"-" gorm:"not null"`
	Action      string `json:"-" gorm:"size:80;not null"`
	Response    string `json:"-" gorm:"type:text;not null"`
	CreatedAt   int64  `json:"-" gorm:"not null;index"`
}

func (ManualRequest) TableName() string { return "payout_manual_request" }

// Usage is platform risk exposure reserved per UTC day, not a bank balance.
type DailyUsage struct {
	ChannelAccountNo string `json:"-" gorm:"size:40;primaryKey"`
	Day              string `json:"-" gorm:"size:10;primaryKey"`
	Amount           int64  `json:"-" gorm:"not null;check:ck_payout_usage,amount>0 AND version>0"`
	Version          int64  `json:"-" gorm:"not null"`
	LastAttemptNo    string `json:"-" gorm:"size:40;not null"`
}

func (DailyUsage) TableName() string { return "payout_channel_daily_usage" }

type Evidence struct {
	EvidenceNo      string `json:"-" gorm:"size:40;primaryKey"`
	PayoutNo        string `json:"-" gorm:"size:40;not null;index"`
	AttemptNo       string `json:"-" gorm:"size:40;not null;index"`
	ObjectReference string `json:"-" gorm:"size:64;not null;uniqueIndex"`
	SHA256          string `json:"-" gorm:"size:64;not null"`
	SourceSHA256    string `json:"-" gorm:"size:64;not null"`
	MIME            string `json:"-" gorm:"size:32;not null"`
	Size            int64  `json:"-" gorm:"not null"`
	EncryptionKeyID string `json:"-" gorm:"size:32;not null;index"`
	CreatedBy       uint64 `json:"-" gorm:"not null;index"`
	CreatedAt       int64  `json:"-" gorm:"not null;index"`
}

func (Evidence) TableName() string { return "payout_evidence" }

type RevealGrant struct {
	GrantNo           string `json:"-" gorm:"size:40;primaryKey"`
	TokenHash         string `json:"-" gorm:"size:64;not null;uniqueIndex"`
	TargetType        string `json:"-" gorm:"size:24;not null"`
	TargetNo          string `json:"-" gorm:"size:40;not null;index"`
	ActorID           uint64 `json:"-" gorm:"not null"`
	Reason            string `json:"-" gorm:"size:500;not null"`
	ExpiresAt         int64  `json:"-" gorm:"not null"`
	ConsumedAt        int64  `json:"-" gorm:"not null"`
	ConsumedRequestID string `json:"-" gorm:"size:64;not null"`
	CreatedAt         int64  `json:"-" gorm:"not null"`
}

func (RevealGrant) TableName() string { return "payout_reveal_grant" }

// This immutable event is a trusted internal completion fact, not a bank callback.
type TerminalEvent struct {
	EventNo        string `json:"eventNo" gorm:"size:40;primaryKey"`
	PayoutNo       string `json:"payoutNo" gorm:"size:40;not null;uniqueIndex"`
	WithdrawalNo   string `json:"withdrawalNo" gorm:"size:40;not null;uniqueIndex"`
	AttemptNo      string `json:"attemptNo,omitempty" gorm:"size:40;not null"`
	TransitionNo   string `json:"transitionNo" gorm:"size:40;not null;uniqueIndex"`
	Kind           string `json:"kind" gorm:"size:24;not null;check:ck_payout_terminal,kind IN ('payout.succeeded','payout.cancelled')"`
	Amount         int64  `json:"amount,string" gorm:"not null"`
	Currency       string `json:"currency" gorm:"size:8;not null"`
	LedgerNo       string `json:"ledgerNo" gorm:"size:40;not null;uniqueIndex"`
	AccountVersion int64  `json:"accountVersion,string" gorm:"not null"`
	ActorID        uint64 `json:"actorId,string" gorm:"not null"`
	CreatedAt      int64  `json:"createdAt" gorm:"not null"`
}

func (TerminalEvent) TableName() string { return "payout_terminal_event" }

type OrderReconciliation struct {
	ReconciliationNo string `json:"reconciliationNo" gorm:"size:40;primaryKey"`
	PayoutNo         string `json:"payoutNo" gorm:"size:40;not null;index"`
	Status           string `json:"status" gorm:"size:16;not null;check:ck_payout_reconciliation,status IN ('passed','abnormal')"`
	Snapshot         string `json:"snapshot" gorm:"type:longtext;not null"`
	SnapshotHash     string `json:"snapshotHash" gorm:"size:64;not null"`
	CreatedBy        uint64 `json:"createdBy,string" gorm:"not null"`
	CreatedAt        int64  `json:"createdAt" gorm:"not null;index"`
}

func (OrderReconciliation) TableName() string { return "payout_reconciliation" }

type Investigation struct {
	InvestigationNo string `json:"investigationNo" gorm:"size:40;primaryKey"`
	PayoutNo        string `json:"payoutNo" gorm:"size:40;not null;index"`
	AttemptNo       string `json:"attemptNo" gorm:"size:40;not null"`
	ActorID         uint64 `json:"actorId,string" gorm:"not null"`
	Note            string `json:"note" gorm:"size:500;not null"`
	EvidenceNo      string `json:"evidenceNo,omitempty" gorm:"size:40;not null"`
	CreatedAt       int64  `json:"createdAt" gorm:"not null"`
}

func (Investigation) TableName() string { return "payout_investigation" }
