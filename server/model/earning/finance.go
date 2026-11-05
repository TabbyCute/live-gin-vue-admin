package earning

// Finance facts deliberately have no soft delete or ORM associations.
type IncomeLock struct {
	AnchorID uint64 `json:"-" gorm:"primaryKey;autoIncrement:false"`
}

func (IncomeLock) TableName() string { return "anchor_income_lock" }

type FinanceControl struct {
	ID uint64 `json:"-" gorm:"primaryKey;autoIncrement:false"`
}

func (FinanceControl) TableName() string { return "anchor_finance_control" }

type Income struct {
	ID               uint64 `json:"-" gorm:"primaryKey"`
	IncomeNo         string `json:"incomeNo" gorm:"size:40;not null;uniqueIndex"`
	AnchorID         uint64 `json:"-" gorm:"not null;index:idx_income_anchor,priority:1"`
	SourceType       string `json:"sourceType" gorm:"size:32;not null;uniqueIndex:uk_income_source,priority:1"`
	SourceNo         string `json:"sourceNo" gorm:"size:128;not null;index"`
	SourceEntryNo    string `json:"sourceEntryNo" gorm:"size:128;not null;uniqueIndex:uk_income_source,priority:2"`
	OriginalIncomeNo string `json:"originalIncomeNo" gorm:"size:40;not null;index"`
	EntryType        string `json:"entryType" gorm:"size:16;not null;check:ck_income_kind,(entry_type='income' AND amount>=0 AND original_income_no='') OR (entry_type='reversal' AND amount<=0 AND original_income_no<>'')"`
	Unit             string `json:"unit" gorm:"size:16;not null"`
	Amount           int64  `json:"amount,string" gorm:"not null"`
	GrossAmount      int64  `json:"grossAmount,string" gorm:"not null"`
	RuleNo           string `json:"ruleNo" gorm:"size:64;not null"`
	ShareBPS         int64  `json:"shareBps,string" gorm:"not null;check:ck_income_bps,share_bps>=0 AND share_bps<=10000"`
	SourceHash       string `json:"-" gorm:"size:64;not null"`
	CreatedAt        int64  `json:"createdAt" gorm:"not null;index:idx_income_anchor,priority:2"`
}

func (Income) TableName() string { return "anchor_income_fact" }

type SettlementRule struct {
	ID            uint64 `json:"-" gorm:"primaryKey"`
	RuleNo        string `json:"ruleNo" gorm:"size:40;not null;uniqueIndex"`
	RequestID     string `json:"requestId" gorm:"size:64;not null;uniqueIndex"`
	RequestHash   string `json:"-" gorm:"size:64;not null"`
	Name          string `json:"name" gorm:"size:80;not null"`
	SourceUnit    string `json:"sourceUnit" gorm:"size:16;not null;index:idx_settlement_rule_window,priority:1"`
	Currency      string `json:"currency" gorm:"size:16;not null;index:idx_settlement_rule_window,priority:2"`
	CoinUnits     int64  `json:"coinUnits,string" gorm:"not null;check:ck_exchange_units,coin_units>0 AND minor_units>0"`
	MinorUnits    int64  `json:"minorUnits,string" gorm:"not null"`
	RoundingMode  string `json:"roundingMode" gorm:"size:16;not null;check:ck_exchange_rounding,rounding_mode='floor'"`
	EffectiveFrom int64  `json:"effectiveFrom" gorm:"not null"`
	EffectiveTo   int64  `json:"effectiveTo" gorm:"not null;check:ck_exchange_window,effective_from>=0 AND effective_to>effective_from"`
	Status        string `json:"status" gorm:"size:16;not null;check:ck_exchange_status,status IN ('pending','active','rejected')"`
	Version       int64  `json:"version,string" gorm:"not null;check:ck_exchange_version,version>0"`
	CreatedBy     uint64 `json:"createdBy,string" gorm:"not null"`
	ApprovedBy    uint64 `json:"approvedBy,string" gorm:"not null"`
	ReviewReason  string `json:"reviewReason" gorm:"size:500;not null"`
	CreatedAt     int64  `json:"createdAt" gorm:"not null"`
	ApprovedAt    int64  `json:"approvedAt" gorm:"not null"`
}

func (SettlementRule) TableName() string { return "anchor_settlement_rule" }

type Settlement struct {
	ID           uint64 `json:"-" gorm:"primaryKey"`
	SettlementNo string `json:"settlementNo" gorm:"size:40;not null;uniqueIndex"`
	RequestID    string `json:"requestId" gorm:"size:64;not null;uniqueIndex"`
	RequestHash  string `json:"-" gorm:"size:64;not null"`
	AnchorID     uint64 `json:"-" gorm:"not null;index:idx_settlement_anchor,priority:1"`
	AnchorNo     string `json:"anchorNo" gorm:"size:40;not null;index"`
	PeriodStart  int64  `json:"periodStart" gorm:"not null"`
	PeriodEnd    int64  `json:"periodEnd" gorm:"not null;check:ck_settlement_window,period_start>=0 AND period_end>period_start"`
	GrossCoin    int64  `json:"grossCoin,string" gorm:"not null"`
	RefundCoin   int64  `json:"refundCoin,string" gorm:"not null"`
	EarningCoin  int64  `json:"earningCoin,string" gorm:"not null"`
	Currency     string `json:"currency" gorm:"size:16;not null"`
	RuleNo       string `json:"ruleNo" gorm:"size:40;not null"`
	RuleSnapshot string `json:"ruleSnapshot" gorm:"type:text;not null"`
	Amount       int64  `json:"amount,string" gorm:"not null"`
	ItemCount    int64  `json:"itemCount,string" gorm:"not null;check:ck_settlement_count,item_count>0"`
	ItemsHash    string `json:"itemsHash" gorm:"size:64;not null"`
	Status       string `json:"status" gorm:"size:16;not null;check:ck_settlement_status,status IN ('pending','approved','rejected')"`
	CreatedBy    uint64 `json:"createdBy,string" gorm:"not null"`
	ApprovedBy   uint64 `json:"approvedBy,string" gorm:"not null"`
	ReviewReason string `json:"reviewReason" gorm:"size:500;not null"`
	CreatedAt    int64  `json:"createdAt" gorm:"not null;index:idx_settlement_anchor,priority:2"`
	ApprovedAt   int64  `json:"approvedAt" gorm:"not null"`
}

func (Settlement) TableName() string { return "anchor_settlement" }

type SettlementItem struct {
	Sequence        int64 `json:"sequence,string" gorm:"not null"`
	RoundingVersion int64 `json:"roundingVersion,string" gorm:"not null"`
	RemainderBefore int64 `json:"remainderBefore,string" gorm:"not null"`
	RemainderAfter  int64 `json:"remainderAfter,string" gorm:"not null"`

	ID           uint64 `json:"-" gorm:"primaryKey"`
	SettlementNo string `json:"settlementNo" gorm:"size:40;not null;uniqueIndex:uk_settlement_item,priority:1"`
	IncomeNo     string `json:"incomeNo" gorm:"size:40;not null;uniqueIndex:uk_settlement_item,priority:2"`
	IncomeHash   string `json:"incomeHash" gorm:"size:64;not null"`
	Amount       int64  `json:"amount,string" gorm:"not null"`
	CoinUnits    int64  `json:"coinUnits,string" gorm:"not null"`
	MinorUnits   int64  `json:"minorUnits,string" gorm:"not null"`
	Currency     string `json:"currency" gorm:"size:16;not null"`
	RuleNo       string `json:"ruleNo" gorm:"size:40;not null"`
}

func (SettlementItem) TableName() string { return "anchor_settlement_item" }

// Only approval creates allocations: pending/rejected previews never consume an income fact.
type Allocation struct {
	IncomeNo     string `json:"incomeNo" gorm:"size:40;primaryKey"`
	SettlementNo string `json:"settlementNo" gorm:"size:40;not null;index"`
	CreatedAt    int64  `json:"createdAt" gorm:"not null"`
}

func (Allocation) TableName() string { return "anchor_settlement_allocation" }

type Account struct {
	ID        uint64 `json:"-" gorm:"primaryKey"`
	AnchorID  uint64 `json:"-" gorm:"not null;uniqueIndex:uk_withdrawable_account,priority:1"`
	AnchorNo  string `json:"anchorNo" gorm:"size:40;not null;index"`
	Currency  string `json:"currency" gorm:"size:16;not null;uniqueIndex:uk_withdrawable_account,priority:2"`
	Available int64  `json:"available,string" gorm:"not null;check:ck_withdrawable_balances_v3,available>=0 AND frozen>=0 AND paid>=0 AND debt>=0 AND (debt=0 OR available=0)"`
	Frozen    int64  `json:"frozen,string" gorm:"not null"`
	Paid      int64  `json:"paid,string" gorm:"not null"`
	Debt      int64  `json:"debt,string" gorm:"not null"`
	Version   int64  `json:"version,string" gorm:"not null;check:ck_withdrawable_version,version>=0"`
	CreatedAt int64  `json:"createdAt" gorm:"not null"`
	UpdatedAt int64  `json:"updatedAt" gorm:"not null"`
}

func (Account) TableName() string { return "anchor_withdrawable_account" }

type AccountLedger struct {
	ID              uint64 `json:"-" gorm:"primaryKey"`
	LedgerNo        string `json:"ledgerNo" gorm:"size:40;not null;uniqueIndex"`
	AnchorID        uint64 `json:"-" gorm:"not null;uniqueIndex:uk_withdrawable_version,priority:1"`
	Currency        string `json:"currency" gorm:"size:16;not null;uniqueIndex:uk_withdrawable_version,priority:2"`
	Version         int64  `json:"version,string" gorm:"not null;uniqueIndex:uk_withdrawable_version,priority:3"`
	EntryType       string `json:"entryType" gorm:"size:32;not null;uniqueIndex:uk_withdrawable_business,priority:1;check:ck_withdrawable_entry_v3,entry_type IN ('settlement_credit','negative_adjustment','withdrawal_freeze','withdrawal_release','withdrawal_paid')"`
	BusinessNo      string `json:"businessNo" gorm:"size:40;not null;uniqueIndex:uk_withdrawable_business,priority:2"`
	ChangeAmount    int64  `json:"changeAmount,string" gorm:"not null"`
	AvailableBefore int64  `json:"availableBefore,string" gorm:"not null"`
	AvailableAfter  int64  `json:"availableAfter,string" gorm:"not null"`
	FrozenBefore    int64  `json:"frozenBefore,string" gorm:"not null"`
	FrozenAfter     int64  `json:"frozenAfter,string" gorm:"not null"`
	DebtBefore      int64  `json:"debtBefore,string" gorm:"not null"`
	DebtAfter       int64  `json:"debtAfter,string" gorm:"not null"`
	PaidBefore      int64  `json:"paidBefore,string" gorm:"not null;default:0;check:ck_withdrawable_paid_chain,paid_before>=0 AND paid_after>=paid_before AND ((entry_type='withdrawal_paid' AND change_amount>0 AND paid_after-paid_before=change_amount AND frozen_before-frozen_after=change_amount AND available_after=available_before AND debt_after=debt_before) OR (entry_type<>'withdrawal_paid' AND paid_after=paid_before))"`
	PaidAfter       int64  `json:"paidAfter,string" gorm:"not null;default:0"`
	CreatedAt       int64  `json:"createdAt" gorm:"not null"`
}

func (AccountLedger) TableName() string { return "anchor_withdrawable_ledger" }

type Withdrawal struct {
	PayoutNo             string `json:"payoutNo,omitempty" gorm:"-"`
	PayoutStatus         string `json:"payoutStatus,omitempty" gorm:"-"`
	ApplicantUserID      uint64 `json:"-" gorm:"not null"`
	ID                   uint64 `json:"-" gorm:"primaryKey"`
	WithdrawalNo         string `json:"withdrawalNo" gorm:"size:40;not null;uniqueIndex"`
	RequestID            string `json:"requestId" gorm:"size:64;not null;uniqueIndex:uk_withdrawal_request,priority:2"`
	RequestHash          string `json:"-" gorm:"size:64;not null"`
	AnchorID             uint64 `json:"-" gorm:"not null;uniqueIndex:uk_withdrawal_request,priority:1;index:idx_withdrawal_anchor,priority:1"`
	AnchorNo             string `json:"anchorNo" gorm:"size:40;not null;index"`
	Amount               int64  `json:"amount,string" gorm:"not null;check:ck_withdrawal_amount,amount>0"`
	Currency             string `json:"currency" gorm:"size:16;not null"`
	PayeeRef             string `json:"payeeRef" gorm:"size:128;not null;index:idx_withdrawal_payee_ref"`
	PayeeVersion         string `json:"payeeVersion" gorm:"size:64;not null"`
	PayeeMethod          string `json:"payeeMethod" gorm:"size:32;not null"`
	AccountName          string `json:"accountName" gorm:"size:128;not null"`
	AccountMasked        string `json:"accountMasked" gorm:"size:128;not null"`
	Status               string `json:"status" gorm:"size:32;not null;index;check:ck_withdrawal_status_v3,status IN ('pending_review','rejected','approved','paid','cancelled')"`
	ReconciliationStatus string `json:"reconciliationStatus" gorm:"size:16;not null;check:ck_withdrawal_recon,reconciliation_status IN ('pending','passed','abnormal')"`
	ReconciliationNo     string `json:"reconciliationNo" gorm:"size:40;not null"`
	ReviewedBy           uint64 `json:"reviewedBy,string" gorm:"not null"`
	ReviewedAt           int64  `json:"reviewedAt" gorm:"not null"`
	ReviewReason         string `json:"reviewReason" gorm:"size:500;not null"`
	CreatedAt            int64  `json:"createdAt" gorm:"not null;index:idx_withdrawal_anchor,priority:2"`
}

func (Withdrawal) TableName() string { return "anchor_withdrawal_order" }

type Reconciliation struct {
	ID               uint64 `json:"-" gorm:"primaryKey"`
	ReconciliationNo string `json:"reconciliationNo" gorm:"size:40;not null;uniqueIndex"`
	WithdrawalNo     string `json:"withdrawalNo" gorm:"size:40;not null;index"`
	AnchorID         uint64 `json:"-" gorm:"not null;index"`
	Currency         string `json:"currency" gorm:"size:16;not null"`
	Snapshot         string `json:"snapshot" gorm:"type:longtext;not null"`
	SnapshotHash     string `json:"snapshotHash" gorm:"size:64;not null"`
	Status           string `json:"status" gorm:"size:16;not null;check:ck_reconciliation_status,status IN ('passed','abnormal')"`
	CreatedBy        uint64 `json:"createdBy,string" gorm:"not null"`
	CreatedAt        int64  `json:"createdAt" gorm:"not null"`
}

func (Reconciliation) TableName() string { return "anchor_reconciliation" }

type FinanceAudit struct {
	ID        uint64 `json:"-" gorm:"primaryKey"`
	ObjectNo  string `json:"objectNo" gorm:"size:40;not null;index"`
	Action    string `json:"action" gorm:"size:40;not null"`
	ActorID   uint64 `json:"actorId,string" gorm:"not null"`
	ActorType string `json:"actorType" gorm:"size:16;not null"`
	ActorIP   string `json:"actorIp" gorm:"size:64;not null"`
	Snapshot  string `json:"snapshot" gorm:"type:text;not null"`
	CreatedAt int64  `json:"createdAt" gorm:"not null"`
}

func (FinanceAudit) TableName() string { return "anchor_finance_audit" }

type AdminPermission struct {
	ID          uint64 `json:"-" gorm:"primaryKey"`
	AuthorityID uint   `json:"authorityId" gorm:"not null;uniqueIndex:uk_earning_permission,priority:1"`
	Path        string `json:"path" gorm:"size:80;not null;uniqueIndex:uk_earning_permission,priority:2"`
	Method      string `json:"method" gorm:"size:8;not null;uniqueIndex:uk_earning_permission,priority:3"`
}

func (AdminPermission) TableName() string { return "anchor_finance_permission" }

// RoundingBalance preserves fractional entitlement across settlement batches at the original rule's rate.
type RoundingBalance struct {
	ID               uint64 `json:"-" gorm:"primaryKey"`
	AnchorID         uint64 `json:"-" gorm:"not null;uniqueIndex:uk_rounding_balance,priority:1"`
	RuleNo           string `json:"ruleNo" gorm:"size:40;not null;uniqueIndex:uk_rounding_balance,priority:2"`
	Currency         string `json:"currency" gorm:"size:16;not null"`
	CoinUnits        int64  `json:"coinUnits,string" gorm:"not null"`
	MinorUnits       int64  `json:"minorUnits,string" gorm:"not null"`
	Remainder        int64  `json:"remainder,string" gorm:"not null;check:ck_rounding_balance,coin_units>0 AND minor_units>0 AND remainder>=0 AND remainder<coin_units AND version>0"`
	Version          int64  `json:"version,string" gorm:"not null"`
	LastSettlementNo string `json:"lastSettlementNo" gorm:"size:40;not null"`
	UpdatedAt        int64  `json:"updatedAt" gorm:"not null"`
}

func (RoundingBalance) TableName() string { return "anchor_settlement_rounding" }
