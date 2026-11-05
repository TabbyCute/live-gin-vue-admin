package ranking

type AppliedEvent struct {
	ID              uint64 `json:"-" gorm:"primaryKey"`
	EventNo         string `json:"eventNo" gorm:"size:40;not null;uniqueIndex"`
	SourceType      string `json:"sourceType" gorm:"size:16;not null"`
	SourceNo        string `json:"sourceNo" gorm:"size:40;not null"`
	SourceID        uint64 `json:"-" gorm:"not null;uniqueIndex"`
	DeltaCoin       int64  `json:"deltaCoin,string" gorm:"not null"`
	DeltaQuantity   int64  `json:"deltaQuantity,string" gorm:"not null"`
	OriginalEventAt int64  `json:"originalEventAt" gorm:"not null"`
	Dimensions      string `json:"-" gorm:"type:text;not null"`
	AppliedAt       int64  `json:"appliedAt" gorm:"not null"`
}

func (AppliedEvent) TableName() string { return "ranking_applied_event" }

// Each generation has independent receipts, so rebuilding does not erase dedupe history.
type Receipt struct {
	ID         uint64 `json:"-" gorm:"primaryKey"`
	Generation uint64 `json:"-" gorm:"not null;uniqueIndex:uk_ranking_receipt,priority:1"`
	SourceID   uint64 `json:"-" gorm:"not null;uniqueIndex:uk_ranking_receipt,priority:2"`
}

func (Receipt) TableName() string { return "ranking_generation_receipt" }

type ScoreBucket struct {
	BucketKey   string `json:"-" gorm:"size:64;not null;uniqueIndex"`
	ID          uint64 `json:"-" gorm:"primaryKey"`
	Generation  uint64 `json:"-" gorm:"not null;uniqueIndex:uk_ranking_bucket,priority:1;index:idx_ranking_board,priority:1"`
	RankType    string `json:"rankType" gorm:"size:8;not null;uniqueIndex:uk_ranking_bucket,priority:2;index:idx_ranking_board,priority:2"`
	PeriodType  string `json:"period" gorm:"size:8;not null;uniqueIndex:uk_ranking_bucket,priority:3;index:idx_ranking_board,priority:3"`
	PeriodKey   string `json:"periodKey" gorm:"size:10;not null;uniqueIndex:uk_ranking_bucket,priority:4;index:idx_ranking_board,priority:4"`
	ScopeType   string `json:"scopeType" gorm:"size:10;not null;uniqueIndex:uk_ranking_bucket,priority:5;index:idx_ranking_board,priority:5"`
	ScopeNo     string `json:"scopeNo" gorm:"size:40;not null;uniqueIndex:uk_ranking_bucket,priority:6;index:idx_ranking_board,priority:6"`
	GiftNo      string `json:"giftNo" gorm:"size:40;not null;uniqueIndex:uk_ranking_bucket,priority:7;index:idx_ranking_board,priority:7"`
	SubjectType string `json:"subjectType" gorm:"size:8;not null;uniqueIndex:uk_ranking_bucket,priority:8"`
	SubjectNo   string `json:"subjectNo" gorm:"size:40;not null;uniqueIndex:uk_ranking_bucket,priority:9;index:idx_ranking_board,priority:9"`
	// Strings and fixed-width sort keys avoid SQL/JS floating point truncation.
	ScoreCoin string `json:"score" gorm:"size:65;not null"`
	Quantity  string `json:"quantity" gorm:"size:65;not null"`
	ScoreSort string `json:"-" gorm:"size:65;not null;index:idx_ranking_board,priority:8,sort:desc"`
	Version   int64  `json:"version,string" gorm:"not null"`
	UpdatedAt int64  `json:"updatedAt" gorm:"not null;autoUpdateTime:false"`
}

func (ScoreBucket) TableName() string { return "ranking_score_bucket" }

type Checkpoint struct {
	ID           uint64 `json:"-" gorm:"primaryKey"`
	ConsumerName string `json:"consumerName" gorm:"size:40;not null;uniqueIndex"`
	LastSourceID uint64 `json:"lastSourceId,string" gorm:"not null"`
	Generation   uint64 `json:"generation,string" gorm:"not null"`
	Timezone     string `json:"timezone" gorm:"size:64;not null"`
	LeaseToken   string `json:"-" gorm:"size:40;not null"`
	LeaseUntil   int64  `json:"leaseUntil" gorm:"not null"`
	LastError    string `json:"lastError" gorm:"size:200;not null"`
	UpdatedAt    int64  `json:"updatedAt" gorm:"not null;autoUpdateTime:false"`
	DataAsOf     int64  `json:"dataAsOf" gorm:"not null"`
}

func (Checkpoint) TableName() string { return "ranking_checkpoint" }

type RebuildJob struct {
	ID             uint64 `json:"-" gorm:"primaryKey"`
	JobNo          string `json:"jobNo" gorm:"size:40;not null;uniqueIndex"`
	Scope          string `json:"scope" gorm:"size:16;not null"`
	Status         string `json:"status" gorm:"size:16;not null;index"`
	Generation     uint64 `json:"generation,string" gorm:"not null"`
	Checkpoint     uint64 `json:"checkpoint,string" gorm:"not null"`
	ProcessedCount uint64 `json:"processedCount,string" gorm:"not null"`
	ErrorCount     uint64 `json:"errorCount,string" gorm:"not null"`
	LastError      string `json:"lastError" gorm:"size:200;not null"`
	StartedAt      int64  `json:"startedAt" gorm:"not null"`
	CompletedAt    int64  `json:"completedAt" gorm:"not null"`
}

func (RebuildJob) TableName() string { return "ranking_rebuild_job" }

type AdminPermission struct {
	ID          uint64 `json:"-" gorm:"primaryKey"`
	AuthorityID uint   `json:"-" gorm:"not null;uniqueIndex:uk_ranking_permission,priority:1"`
	Path        string `json:"path" gorm:"size:100;not null;uniqueIndex:uk_ranking_permission,priority:2"`
	Method      string `json:"method" gorm:"size:10;not null;uniqueIndex:uk_ranking_permission,priority:3"`
}

func (AdminPermission) TableName() string { return "ranking_admin_permission" }

type Audit struct {
	ID         uint64 `json:"-" gorm:"primaryKey"`
	RequestID  string `json:"requestId" gorm:"size:64;not null;uniqueIndex"`
	OperatorID uint64 `json:"-" gorm:"not null"`
	Action     string `json:"action" gorm:"size:64;not null"`
	Input      string `json:"-" gorm:"type:text;not null"`
	Result     string `json:"result" gorm:"type:longtext;not null"`
	ActorIP    string `json:"-" gorm:"size:45;not null"`
	CreatedAt  int64  `json:"createdAt" gorm:"not null"`
}

func (Audit) TableName() string { return "ranking_audit_log" }
