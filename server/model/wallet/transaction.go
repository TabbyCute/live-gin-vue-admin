package wallet

import "gorm.io/gorm"

const (
	BusinessDebit     = "CONSUME"
	BusinessRefund    = "CONSUME_REFUND"
	TransactionDebit  = "debit"
	TransactionRefund = "refund"
)

// WalletTransaction 保存服务端已核实的消费/退款事实。仅原扣款的累计退款额可更新。
// 不保存商品、权益或价格规则；业务模块与本表可在同一数据库事务提交。
type WalletTransaction struct {
	ID             uint64 `gorm:"primaryKey;autoIncrement;index:idx_wallet_transaction_account,priority:2;index:idx_wallet_transaction_original,priority:2"`
	TransactionNo  string `gorm:"type:varchar(40);not null;uniqueIndex:uk_wallet_transaction_no"`
	Kind           string `gorm:"type:varchar(16);not null;uniqueIndex:uk_wallet_transaction_business,priority:1;check:ck_wallet_transaction_kind,kind IN ('debit','refund')"`
	BusinessType   string `gorm:"type:varchar(32);not null;uniqueIndex:uk_wallet_transaction_business,priority:2"`
	BusinessNo     string `gorm:"type:varchar(128);not null;uniqueIndex:uk_wallet_transaction_business,priority:3"`
	AccountID      uint64 `gorm:"not null;index:idx_wallet_transaction_account,priority:1"`
	AccountNo      string `gorm:"type:varchar(32);not null"`
	CurrencyCode   string `gorm:"type:varchar(16);not null"`
	Amount         int64  `gorm:"not null;check:ck_wallet_transaction_amount,amount > 0 AND refunded_amount >= 0 AND refunded_amount <= amount AND (kind = 'debit' OR refunded_amount = 0)"`
	RefundedAmount int64  `gorm:"not null;default:0"`
	OriginalNo     string `gorm:"type:varchar(40);not null;default:'';index:idx_wallet_transaction_original,priority:1;check:ck_wallet_transaction_original,(kind = 'debit' AND original_no = '') OR (kind = 'refund' AND original_no <> '')"`
	LedgerNo       string `gorm:"type:varchar(40);not null;uniqueIndex:uk_wallet_transaction_ledger"`
	RequestHash    string `gorm:"type:char(64);not null"`
	ActorType      string `gorm:"type:varchar(16);not null;check:ck_wallet_transaction_actor,actor_type IN ('user','admin','system')"`
	ActorID        string `gorm:"type:varchar(64);not null"`
	ActorIP        string `gorm:"type:varchar(45);not null"`
	TraceID        string `gorm:"type:varchar(64);not null"`
	Reason         string `gorm:"type:varchar(500);not null"`
	CreatedAt      int64  `gorm:"autoCreateTime:milli;not null"`
}

func (WalletTransaction) TableName() string            { return "wallet_transaction" }
func (*WalletTransaction) BeforeDelete(*gorm.DB) error { return ErrImmutable }
