// Package bankreconcile contains append-only external assertions, never monetary facts.
package bankreconcile

type Batch struct {
	BatchNo          string `json:"batchNo" gorm:"size:40;primaryKey"`
	ChannelAccountNo string `json:"channelAccountNo" gorm:"size:40;not null;uniqueIndex:uk_bank_batch_source,priority:1;index"`
	DeclaredComplete bool   `json:"declaredComplete" gorm:"not null"`
	SourceHash       string `json:"sourceHash" gorm:"size:64;not null;uniqueIndex:uk_bank_batch_source,priority:2"`
	CompanyMasked    string `json:"companyMasked" gorm:"size:128;not null"`
	Currency         string `json:"currency" gorm:"size:8;not null"`
	PeriodFrom       int64  `json:"periodFrom" gorm:"not null;check:ck_bank_period,period_from>0 AND period_to>period_from"`
	PeriodTo         int64  `json:"periodTo" gorm:"not null"`
	RowCount         int    `json:"rowCount" gorm:"not null;check:ck_bank_rows,row_count>0 AND row_count<=1000"`
	CreatedBy        uint64 `json:"createdBy,string" gorm:"not null"`
	CreatedAt        int64  `json:"createdAt" gorm:"not null;autoCreateTime:false;index"`
}

func (Batch) TableName() string { return "payout_bank_batch" }

type Line struct {
	LineNo               string `json:"lineNo" gorm:"size:40;primaryKey"`
	ChannelAccountNo     string `json:"channelAccountNo" gorm:"size:40;not null;uniqueIndex:uk_bank_line_ref,priority:1"`
	ReferenceFingerprint string `json:"-" gorm:"size:64;not null;uniqueIndex:uk_bank_line_ref,priority:2"`
	ReferenceMasked      string `json:"referenceMasked" gorm:"size:132;not null"`
	RecipientFingerprint string `json:"-" gorm:"size:64;not null"`
	ContentHash          string `json:"-" gorm:"size:64;not null"`
	Currency             string `json:"currency" gorm:"size:8;not null"`
	Amount               int64  `json:"amount,string" gorm:"not null;check:ck_bank_amount,amount>0"`
	BankStatus           string `json:"bankStatus" gorm:"size:16;not null;check:ck_bank_status,bank_status IN ('settled','pending','failed','reversed')"`
	BookedAt             int64  `json:"bookedAt" gorm:"not null"`
}

func (Line) TableName() string { return "payout_bank_line" }

type BatchLine struct {
	BatchNo string `json:"batchNo" gorm:"size:40;primaryKey"`
	LineNo  string `json:"lineNo" gorm:"size:40;primaryKey"`
}

func (BatchLine) TableName() string { return "payout_bank_batch_line" }

type Run struct {
	RunNo      string `json:"runNo" gorm:"size:40;primaryKey"`
	BatchNo    string `json:"batchNo" gorm:"size:40;not null;index"`
	CreatedBy  uint64 `json:"createdBy,string" gorm:"not null"`
	CreatedAt  int64  `json:"createdAt" gorm:"not null;autoCreateTime:false"`
	CheckCount int    `json:"checkCount" gorm:"not null"`
	IssueCount int    `json:"issueCount" gorm:"not null"`
}

func (Run) TableName() string { return "payout_bank_run" }

type Check struct {
	CheckNo        string `json:"checkNo" gorm:"size:40;primaryKey"`
	RunNo          string `json:"runNo" gorm:"size:40;not null;index"`
	LineNo         string `json:"lineNo" gorm:"size:40;not null"`
	PayoutNo       string `json:"payoutNo" gorm:"size:40;not null"`
	AttemptNo      string `json:"attemptNo" gorm:"size:40;not null"`
	AttemptVersion int64  `json:"attemptVersion,string" gorm:"not null"`
	Result         string `json:"result" gorm:"size:32;not null"`
}

func (Check) TableName() string { return "payout_bank_check" }

type Review struct {
	ReviewNo      string `json:"reviewNo" gorm:"size:40;primaryKey"`
	CheckNo       string `json:"checkNo" gorm:"size:40;not null;uniqueIndex"`
	Decision      string `json:"decision" gorm:"size:24;not null;check:ck_bank_review,decision IN ('acknowledge','investigate')"`
	Reason        string `json:"reason" gorm:"size:500;not null"`
	ActorID       uint64 `json:"actorId,string" gorm:"not null"`
	RootException bool   `json:"rootException" gorm:"not null"`
	CreatedAt     int64  `json:"createdAt" gorm:"not null;autoCreateTime:false"`
}

func (Review) TableName() string { return "payout_bank_review" }

type Receipt struct {
	RequestID string `json:"-" gorm:"size:64;primaryKey"`
	ActorID   uint64 `json:"-" gorm:"not null"`
	Action    string `json:"-" gorm:"size:16;not null"`
	Digest    string `json:"-" gorm:"size:64;not null"`
	ResultNo  string `json:"-" gorm:"size:40;not null"`
	CreatedAt int64  `json:"-" gorm:"not null;autoCreateTime:false"`
}

func (Receipt) TableName() string { return "payout_bank_request" }

type Audit struct {
	EventNo   string `json:"eventNo" gorm:"size:40;primaryKey"`
	Action    string `json:"action" gorm:"size:16;not null"`
	ObjectNo  string `json:"objectNo" gorm:"size:40;not null;index"`
	ActorID   uint64 `json:"actorId,string" gorm:"not null"`
	TraceID   string `json:"traceId" gorm:"size:64;not null"`
	CreatedAt int64  `json:"createdAt" gorm:"not null;autoCreateTime:false"`
}

func (Audit) TableName() string { return "payout_bank_audit" }
