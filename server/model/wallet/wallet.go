// Package wallet 定义账务事实；不嵌入带软删除的GVA_MODEL，任何账务事实均不得删除。
package wallet

import (
	"errors"
	"gorm.io/gorm"
)

const (
	CurrencyCOIN    = "COIN"
	AccountNormal   = "normal"
	AccountDisabled = "disabled"
	KindGrant       = "grant"
	KindReverse     = "reverse"
	StatusPending   = "pending"
	StatusCredited  = "credited"
	StatusRejected  = "rejected"
	StatusReversed  = "reversed"
	BusinessGrant   = "ADMIN_GRANT"
	BusinessReverse = "ADMIN_REVERSE"
	// manual 为真实资金人工充值；legacy/reversal 仅为历史迁移及内部冲正命名空间。
	SourceManual   = "manual"
	SourceLegacy   = "legacy"
	SourceReversal = "reversal"
)

// 所有金额为int64最小单位；对外DTO以十进制字符串输出，避免JavaScript精度损失。
type WalletAccount struct {
	ID               uint64 `json:"-" gorm:"column:id;primaryKey;autoIncrement"`
	AccountID        uint64 `json:"-" gorm:"column:account_id;not null;uniqueIndex:uk_wallet_account_currency,priority:1"`
	CurrencyCode     string `json:"-" gorm:"column:currency_code;type:varchar(16);not null;uniqueIndex:uk_wallet_account_currency,priority:2"`
	AvailableBalance int64  `json:"-" gorm:"column:available_balance;not null;default:0;check:ck_wallet_account_balance,available_balance >= 0 AND frozen_balance >= 0 AND version >= 0"`
	FrozenBalance    int64  `json:"-" gorm:"column:frozen_balance;not null;default:0;comment:保留字段，当前固定为0"`
	Version          int64  `json:"-" gorm:"column:version;not null;default:0;comment:账务流水连续序号"`
	Status           string `json:"-" gorm:"column:status;type:varchar(16);not null;default:normal;check:ck_wallet_account_status,status IN ('normal','disabled')"`
	CreatedAt        int64  `json:"-" gorm:"column:created_at;autoCreateTime:milli;not null"`
	UpdatedAt        int64  `json:"-" gorm:"column:updated_at;autoUpdateTime:milli;not null"`
}

func (WalletAccount) TableName() string { return "wallet_account" }

// WalletLedger 仅追加。WalletVersion让每个钱包具有可校验的连续链。
type WalletLedger struct {
	ID               uint64 `json:"-" gorm:"column:id;primaryKey;autoIncrement"`
	LedgerNo         string `json:"-" gorm:"column:ledger_no;type:varchar(40);not null;uniqueIndex:uk_wallet_ledger_no"`
	WalletAccountID  uint64 `json:"-" gorm:"column:wallet_account_id;not null;uniqueIndex:uk_wallet_ledger_version,priority:1"`
	WalletVersion    int64  `json:"-" gorm:"column:wallet_version;not null;uniqueIndex:uk_wallet_ledger_version,priority:2"`
	ChangeAmount     int64  `json:"-" gorm:"column:change_amount;not null;check:ck_wallet_ledger_amount,change_amount <> 0 AND balance_before >= 0 AND balance_after >= 0 AND balance_after - balance_before = change_amount AND wallet_version > 0"`
	BalanceBefore    int64  `json:"-" gorm:"column:balance_before;not null"`
	BalanceAfter     int64  `json:"-" gorm:"column:balance_after;not null"`
	BusinessType     string `json:"-" gorm:"column:business_type;type:varchar(32);not null;uniqueIndex:uk_wallet_ledger_business,priority:1"`
	BusinessNo       string `json:"-" gorm:"column:business_no;type:varchar(40);not null;uniqueIndex:uk_wallet_ledger_business,priority:2"`
	OriginalLedgerNo string `json:"-" gorm:"column:original_ledger_no;type:varchar(40);not null;default:''"`
	OperatorID       uint64 `json:"-" gorm:"column:operator_id;not null"`
	Reason           string `json:"-" gorm:"column:reason;type:varchar(500);not null"`
	CreatedAt        int64  `json:"-" gorm:"column:created_at;autoCreateTime:milli;not null"`
}

func (WalletLedger) TableName() string { return "wallet_ledger" }

var ErrImmutable = errors.New("wallet accounting records are immutable")

func (*WalletLedger) BeforeUpdate(*gorm.DB) error { return ErrImmutable }
func (*WalletLedger) BeforeDelete(*gorm.DB) error { return ErrImmutable }

// WalletAdjustmentOrder 入账与冲正共用审核工作流；冲正关联原单且全额冲正。
type WalletAdjustmentOrder struct {
	ID              uint64 `json:"-" gorm:"column:id;primaryKey;autoIncrement;index:idx_wallet_order_account,priority:2;index:idx_wallet_order_status,priority:2"`
	OrderNo         string `json:"-" gorm:"column:order_no;type:varchar(40);not null;uniqueIndex:uk_wallet_order_no"`
	RequestID       string `json:"-" gorm:"column:request_id;type:varchar(64);not null;uniqueIndex:uk_wallet_order_request,priority:2"`
	RequestHash     string `json:"-" gorm:"column:request_hash;type:char(64);not null"`
	SourceType      string `json:"-" gorm:"column:source_type;type:varchar(16);not null;default:legacy;uniqueIndex:uk_wallet_order_source,priority:1"`
	ExternalOrderNo string `json:"-" gorm:"column:external_order_no;type:varchar(128);not null;default:'';uniqueIndex:uk_wallet_order_source,priority:2;check:ck_wallet_order_source,external_order_no <> '' AND ((kind = 'grant' AND source_type IN ('manual','legacy')) OR (kind = 'reverse' AND source_type = 'reversal'))"`
	Kind            string `json:"-" gorm:"column:kind;type:varchar(16);not null;check:ck_wallet_order_kind,kind IN ('grant','reverse')"`
	AccountID       uint64 `json:"-" gorm:"column:account_id;not null;index:idx_wallet_order_account,priority:1"`
	AccountNo       string `json:"-" gorm:"column:account_no;type:varchar(32);not null;comment:创建时公开账号快照"`
	CurrencyCode    string `json:"-" gorm:"column:currency_code;type:varchar(16);not null"`
	Amount          int64  `json:"-" gorm:"column:amount;not null;check:ck_wallet_order_amount,amount > 0"`
	Reason          string `json:"-" gorm:"column:reason;type:varchar(500);not null"`
	EvidenceRef     string `json:"-" gorm:"column:evidence_ref;type:varchar(500);not null;default:'';comment:已上传凭证引用，不访问外部URL"`
	Status          string `json:"-" gorm:"column:status;type:varchar(16);not null;index:idx_wallet_order_status,priority:1;index:idx_wallet_order_pending,priority:2;check:ck_wallet_order_status,status IN ('pending','credited','rejected','reversed')"`
	OriginalOrderNo string `json:"-" gorm:"column:original_order_no;type:varchar(40);not null;default:'';index:idx_wallet_order_original"`
	ReversalOrderNo string `json:"-" gorm:"column:reversal_order_no;type:varchar(40);not null;default:'';comment:当前有效冲正申请，驳回后释放"`
	LedgerNo        string `json:"-" gorm:"column:ledger_no;type:varchar(40);not null;default:''"`
	CreatedBy       uint64 `json:"-" gorm:"column:created_by;not null;uniqueIndex:uk_wallet_order_request,priority:1;index:idx_wallet_order_pending,priority:1"`
	ApprovedBy      uint64 `json:"-" gorm:"column:approved_by;not null;default:0"`
	RejectedBy      uint64 `json:"-" gorm:"column:rejected_by;not null;default:0"`
	ReviewReason    string `json:"-" gorm:"column:review_reason;type:varchar(500);not null;default:''"`
	CreatedIP       string `json:"-" gorm:"column:created_ip;type:varchar(45);not null"`
	ReviewedIP      string `json:"-" gorm:"column:reviewed_ip;type:varchar(45);not null;default:''"`
	ApprovedAt      int64  `json:"-" gorm:"column:approved_at;not null;default:0"`
	CreditedAt      int64  `json:"-" gorm:"column:credited_at;not null;default:0"`
	RejectedAt      int64  `json:"-" gorm:"column:rejected_at;not null;default:0"`
	ReversedAt      int64  `json:"-" gorm:"column:reversed_at;not null;default:0"`
	CreatedAt       int64  `json:"-" gorm:"column:created_at;autoCreateTime:milli;not null"`
	UpdatedAt       int64  `json:"-" gorm:"column:updated_at;autoUpdateTime:milli;not null"`
}

func (WalletAdjustmentOrder) TableName() string            { return "wallet_adjustment_order" }
func (*WalletAdjustmentOrder) BeforeDelete(*gorm.DB) error { return ErrImmutable }

// WalletDailyQuota 按UTC自然日记录累计入账，冲正不返还限额，防止循环赠送绕过。
type WalletDailyQuota struct {
	ID           uint64 `json:"-" gorm:"column:id;primaryKey;autoIncrement"`
	Day          string `json:"-" gorm:"column:day;type:char(10);not null;uniqueIndex:uk_wallet_quota,priority:1"`
	SubjectType  string `json:"-" gorm:"column:subject_type;type:varchar(16);not null;uniqueIndex:uk_wallet_quota,priority:2"`
	SubjectID    uint64 `json:"-" gorm:"column:subject_id;not null;uniqueIndex:uk_wallet_quota,priority:3"`
	CurrencyCode string `json:"-" gorm:"column:currency_code;type:varchar(16);not null;uniqueIndex:uk_wallet_quota,priority:4"`
	Amount       int64  `json:"-" gorm:"column:amount;not null;default:0;check:ck_wallet_quota_amount,amount >= 0"`
	UpdatedAt    int64  `json:"-" gorm:"column:updated_at;autoUpdateTime:milli;not null"`
}

func (WalletDailyQuota) TableName() string { return "wallet_daily_quota" }

type WalletAuditLog struct {
	ID         uint64 `json:"-" gorm:"column:id;primaryKey;autoIncrement;index:idx_wallet_audit_object,priority:2"`
	ObjectNo   string `json:"-" gorm:"column:object_no;type:varchar(64);not null;index:idx_wallet_audit_object,priority:1"`
	Action     string `json:"-" gorm:"column:action;type:varchar(32);not null"`
	OperatorID uint64 `json:"-" gorm:"column:operator_id;not null"`
	OperatorIP string `json:"-" gorm:"column:operator_ip;type:varchar(45);not null"`
	Reason     string `json:"-" gorm:"column:reason;type:varchar(500);not null"`
	CreatedAt  int64  `json:"-" gorm:"column:created_at;autoCreateTime:milli;not null"`
}

func (WalletAuditLog) TableName() string            { return "wallet_audit_log" }
func (*WalletAuditLog) BeforeUpdate(*gorm.DB) error { return ErrImmutable }
func (*WalletAuditLog) BeforeDelete(*gorm.DB) error { return ErrImmutable }
func (*WalletAccount) BeforeDelete(*gorm.DB) error  { return ErrImmutable }
func Models() []interface{} {
	return []interface{}{&WalletAccount{}, &WalletLedger{}, &WalletAdjustmentOrder{}, &WalletDailyQuota{}, &WalletAuditLog{}}
}
