package payout

// PayeeAccount is an immutable binding. Only lifecycle/default fields may change.
// Explicit hidden JSON tags prevent accidental serialization of storage material.
type PayeeAccount struct {
	ID                 uint64 `json:"-" gorm:"primaryKey"`
	PayeeAccountNo     string `json:"-" gorm:"size:40;not null;uniqueIndex"`
	OwnerType          string `json:"-" gorm:"size:16;not null;uniqueIndex:uk_payee_fingerprint,priority:1;uniqueIndex:uk_payee_request,priority:1;uniqueIndex:uk_payee_default,priority:1;index:idx_payee_owner,priority:1;check:ck_payee_owner,owner_type='anchor'"`
	OwnerNo            string `json:"-" gorm:"size:40;not null;uniqueIndex:uk_payee_fingerprint,priority:2;uniqueIndex:uk_payee_request,priority:2;uniqueIndex:uk_payee_default,priority:2;index:idx_payee_owner,priority:2"`
	Method             string `json:"-" gorm:"size:16;not null;uniqueIndex:uk_payee_fingerprint,priority:3;check:ck_payee_method,method IN ('bank_card','alipay','wechat')"`
	Currency           string `json:"-" gorm:"size:8;not null;uniqueIndex:uk_payee_fingerprint,priority:4;uniqueIndex:uk_payee_default,priority:3;index:idx_payee_owner,priority:3"`
	EncryptedPayload   string `json:"-" gorm:"type:text;not null"`
	EncryptionKeyID    string `json:"-" gorm:"size:32;not null;index:idx_payee_key"`
	PayloadVersion     int    `json:"-" gorm:"not null;check:ck_payee_payload,payload_version=1 AND version=1"`
	MaskedAccount      string `json:"-" gorm:"size:128;not null"`
	AccountFingerprint string `json:"-" gorm:"size:64;not null;uniqueIndex:uk_payee_fingerprint,priority:5"`
	RequestID          string `json:"-" gorm:"size:64;not null;uniqueIndex:uk_payee_request,priority:3"`
	RequestHash        string `json:"-" gorm:"size:64;not null"`
	Status             string `json:"-" gorm:"size:16;not null;index:idx_payee_owner,priority:4;check:ck_payee_status,status IN ('active','disabled')"`
	Version            int64  `json:"-" gorm:"not null"`
	// NULL permits many non-default accounts; 1 permits exactly one per owner/currency.
	DefaultSlot *int  `json:"-" gorm:"uniqueIndex:uk_payee_default,priority:4;check:ck_payee_default,default_slot IS NULL OR (default_slot=1 AND status='active')"`
	CreatedAt   int64 `json:"-" gorm:"not null;autoCreateTime:false"`
	UpdatedAt   int64 `json:"-" gorm:"not null;autoUpdateTime:false;check:ck_payee_time,created_at>0 AND updated_at>=created_at AND ((status='active' AND disabled_at=0) OR (status='disabled' AND disabled_at>=created_at))"`
	DisabledAt  int64 `json:"-" gorm:"not null"`
}

func (PayeeAccount) TableName() string { return "payout_payee_account" }

// OwnerLock serializes creation/default/disable and withdrawal snapshot selection.
type OwnerLock struct {
	OwnerType string `json:"-" gorm:"size:16;primaryKey"`
	OwnerNo   string `json:"-" gorm:"size:40;primaryKey"`
}

func (OwnerLock) TableName() string { return "payout_owner_lock" }

type Audit struct {
	ID             uint64 `json:"-" gorm:"primaryKey"`
	EventNo        string `json:"-" gorm:"size:40;not null;uniqueIndex"`
	PayeeAccountNo string `json:"-" gorm:"size:40;not null;index"`
	OwnerNo        string `json:"-" gorm:"size:40;not null;index:idx_payee_audit_owner,priority:1"`
	ActorID        uint64 `json:"-" gorm:"not null"`
	Action         string `json:"-" gorm:"size:32;not null"`
	TraceID        string `json:"-" gorm:"size:40;not null;index"`
	Snapshot       string `json:"-" gorm:"type:text;not null"`
	CreatedAt      int64  `json:"-" gorm:"not null;index:idx_payee_audit_owner,priority:2"`
}

func (Audit) TableName() string { return "payout_payee_audit" }
