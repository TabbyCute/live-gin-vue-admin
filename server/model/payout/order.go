package payout

// Order is an immutable instruction awaiting a future, separately authorized
// payment state machine. No ORM association can cascade into financial facts.
type Order struct {
	StateVersion           int64  `json:"-" gorm:"not null;default:1;check:ck_payout_state_version,state_version>0"`
	LastTransitionNo       string `json:"-" gorm:"size:40;not null;default:''"`
	ID                     uint64 `json:"-" gorm:"primaryKey"`
	PayoutNo               string `json:"-" gorm:"size:40;not null;uniqueIndex"`
	RequestID              string `json:"-" gorm:"size:64;not null;uniqueIndex"`
	RequestHash            string `json:"-" gorm:"size:64;not null"`
	BusinessType           string `json:"-" gorm:"size:32;not null;uniqueIndex:uk_payout_business,priority:1;check:ck_payout_business,business_type='anchor_withdrawal'"`
	BusinessNo             string `json:"-" gorm:"size:40;not null;uniqueIndex:uk_payout_business,priority:2"`
	AnchorID               uint64 `json:"-" gorm:"not null"`
	AnchorNo               string `json:"-" gorm:"size:40;not null;index:idx_payout_anchor_time,priority:1"`
	ApplicantUserID        uint64 `json:"-" gorm:"not null"`
	Amount                 int64  `json:"-" gorm:"not null;check:ck_payout_amount,amount>0"`
	Currency               string `json:"-" gorm:"size:8;not null;index:idx_payout_currency_time,priority:1"`
	PayeeAccountNo         string `json:"-" gorm:"size:40;not null;index"`
	PayeeVersion           string `json:"-" gorm:"size:64;not null"`
	PayeeMethod            string `json:"-" gorm:"size:16;not null;index:idx_payout_method_time,priority:1;check:ck_payout_method,payee_method IN ('bank_card','alipay','wechat')"`
	AccountName            string `json:"-" gorm:"size:128;not null"`
	AccountMasked          string `json:"-" gorm:"size:128;not null"`
	PayeeSnapshotEncrypted string `json:"-" gorm:"type:text;not null"`
	PayeeSnapshotHash      string `json:"-" gorm:"size:64;not null"`
	EncryptionKeyID        string `json:"-" gorm:"size:32;not null;index"`
	PayloadVersion         int    `json:"-" gorm:"not null;check:ck_payout_payload,payload_version=1"`
	ReconciliationSnapshot string `json:"-" gorm:"type:longtext;not null"`
	Status                 string `json:"-" gorm:"size:24;not null;index:idx_payout_status_time,priority:1;check:ck_payout_status_v3,status IN ('ready','processing','pending_confirmation','succeeded','failed','unknown','cancelled')"`
	CreatedBy              uint64 `json:"-" gorm:"not null;check:ck_payout_actor,created_by>0"`
	CreatedAt              int64  `json:"-" gorm:"not null;autoCreateTime:false;index:idx_payout_time;index:idx_payout_anchor_time,priority:2;index:idx_payout_currency_time,priority:2;index:idx_payout_method_time,priority:2;index:idx_payout_status_time,priority:2"`
	UpdatedAt              int64  `json:"-" gorm:"not null;autoUpdateTime:false;check:ck_payout_time_v3,created_at>0 AND updated_at>=created_at"`
}

func (Order) TableName() string { return "payout_order" }

// Every accepted request key, including a second key for the same withdrawal,
// remains bound forever. Otherwise that second key could later change meaning.
type OrderRequest struct {
	RequestID   string `json:"-" gorm:"size:64;primaryKey"`
	RequestHash string `json:"-" gorm:"size:64;not null"`
	PayoutNo    string `json:"-" gorm:"size:40;not null;index"`
	CreatedBy   uint64 `json:"-" gorm:"not null"`
	CreatedAt   int64  `json:"-" gorm:"not null;autoCreateTime:false"`
}

func (OrderRequest) TableName() string { return "payout_order_request" }

type StatusHistory struct {
	ID         uint64 `json:"-" gorm:"primaryKey"`
	EventNo    string `json:"eventNo" gorm:"size:40;not null;uniqueIndex"`
	PayoutNo   string `json:"payoutNo" gorm:"size:40;not null;uniqueIndex:uk_payout_history_sequence,priority:1"`
	Sequence   int64  `json:"sequence,string" gorm:"not null;default:1;uniqueIndex:uk_payout_history_sequence,priority:2;check:ck_payout_history_sequence,sequence>0"`
	AttemptNo  string `json:"attemptNo,omitempty" gorm:"size:40;not null;default:'';index"`
	RequestID  string `json:"requestId,omitempty" gorm:"size:64;not null;default:''"`
	Reason     string `json:"reason,omitempty" gorm:"size:500;not null;default:''"`
	FromStatus string `json:"fromStatus" gorm:"size:24;not null"`
	ToStatus   string `json:"toStatus" gorm:"size:24;not null;check:ck_payout_history_v3,(from_status='' AND to_status='ready') OR (from_status='ready' AND to_status IN ('processing','cancelled')) OR (from_status='processing' AND to_status IN ('ready','pending_confirmation','failed')) OR (from_status='pending_confirmation' AND to_status IN ('succeeded','failed','unknown')) OR (from_status='unknown' AND to_status IN ('succeeded','failed')) OR (from_status='failed' AND to_status IN ('processing','cancelled'))"`
	ActorID    uint64 `json:"actorId,string" gorm:"not null"`
	CreatedAt  int64  `json:"createdAt" gorm:"not null;autoCreateTime:false"`
}

func (StatusHistory) TableName() string { return "payout_status_history" }

type OrderAudit struct {
	TransitionNo string `json:"transitionNo,omitempty" gorm:"size:40;not null;default:'';index"`
	ObjectNo     string `json:"objectNo,omitempty" gorm:"size:40;not null;default:'';index"`
	ID           uint64 `json:"-" gorm:"primaryKey"`
	EventNo      string `json:"eventNo" gorm:"size:40;not null;uniqueIndex"`
	PayoutNo     string `json:"payoutNo" gorm:"size:40;not null;index"`
	Action       string `json:"action" gorm:"size:40;not null;index;check:ck_payout_audit_v3,action IN ('order.create','order.start','order.retry','order.release','attempt.bank_submitted','attempt.submit','attempt.review_success','attempt.review_failure','attempt.review_unknown','attempt.resolve_unknown','attempt.fail_unpaid','order.cancel','order.investigate','order.reconcile','channel_account.create','channel_account.enable','channel_account.disable','channel_account.reveal','payee.reveal_grant','payee.reveal','evidence.upload','evidence.read')"`
	ActorID      uint64 `json:"actorId,string" gorm:"not null"`
	TraceID      string `json:"traceId" gorm:"size:40;not null;index"`
	Snapshot     string `json:"snapshot" gorm:"type:text;not null"`
	CreatedAt    int64  `json:"createdAt" gorm:"not null;autoCreateTime:false"`
}

func (OrderAudit) TableName() string { return "payout_audit_log" }
