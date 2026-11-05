package guard

// Financial/entitlement facts deliberately have no soft deletion.
type Plan struct {
	ID                  uint64 `json:"-" gorm:"primaryKey"`
	PlanNo              string `json:"planNo" gorm:"size:40;not null;uniqueIndex"`
	Name                string `json:"name" gorm:"size:64;not null"`
	Level               int    `json:"level" gorm:"not null"`
	Priority            int    `json:"priority" gorm:"not null"`
	DefaultDurationDays int64  `json:"defaultDurationDays,string" gorm:"not null;check:ck_guard_plan_days,default_duration_days BETWEEN 1 AND 3650"`
	BenefitConfig       string `json:"benefitConfig" gorm:"type:text;not null"`
	Status              string `json:"status" gorm:"size:16;not null;check:ck_guard_plan_status,status IN ('enabled','disabled')"`
	SortOrder           int    `json:"sortOrder" gorm:"not null"`
	Version             int64  `json:"version,string" gorm:"not null"`
	CreatedBy           uint64 `json:"-" gorm:"not null"`
	UpdatedBy           uint64 `json:"-" gorm:"not null"`
	CreatedAt           int64  `json:"createdAt" gorm:"not null"`
	UpdatedAt           int64  `json:"updatedAt" gorm:"not null;autoUpdateTime:false"`
}

func (Plan) TableName() string { return "guard_plan" }

type Membership struct {
	ID               uint64 `json:"-" gorm:"primaryKey"`
	MembershipNo     string `json:"membershipNo" gorm:"size:40;not null;uniqueIndex"`
	UserID           uint64 `json:"-" gorm:"not null;uniqueIndex:uk_guard_member,priority:1"`
	AnchorID         uint64 `json:"-" gorm:"not null;uniqueIndex:uk_guard_member,priority:2;index"`
	ActivePlanNo     string `json:"activePlanNo" gorm:"size:40;not null"`
	StartedAt        int64  `json:"startedAt" gorm:"not null"`
	ExpiresAt        int64  `json:"expiresAt" gorm:"not null"`
	NextTransitionAt int64  `json:"-" gorm:"not null;index:idx_guard_due,priority:2"`
	Status           string `json:"status" gorm:"size:16;not null;index:idx_guard_due,priority:1"`
	Version          int64  `json:"version,string" gorm:"not null"`
	Sequence         int64  `json:"-" gorm:"not null"`
	Projection       string `json:"-" gorm:"type:longtext;not null"`
	LedgerNo         string `json:"-" gorm:"size:40;not null"`
	ValidUntil       int64  `json:"-" gorm:"not null;index"`
	UpdatedAt        int64  `json:"updatedAt" gorm:"not null;autoUpdateTime:false"`
}

func (Membership) TableName() string { return "guard_membership" }

type Grant struct {
	ID                  uint64 `json:"-" gorm:"primaryKey"`
	GrantNo             string `json:"grantNo" gorm:"size:40;not null;uniqueIndex"`
	MembershipNo        string `json:"membershipNo" gorm:"size:40;not null;index"`
	SourceType          string `json:"sourceType" gorm:"size:32;not null;uniqueIndex:uk_guard_grant_source,priority:1"`
	SourceNo            string `json:"sourceNo" gorm:"size:40;not null;uniqueIndex:uk_guard_grant_source,priority:2"`
	SourceEntryNo       string `json:"sourceEntryNo" gorm:"size:40;not null;uniqueIndex:uk_guard_grant_source,priority:3"`
	UserID              uint64 `json:"-" gorm:"not null"`
	AnchorID            uint64 `json:"-" gorm:"not null"`
	PlanNo              string `json:"planNo" gorm:"size:40;not null;index"`
	PlanSnapshot        string `json:"planSnapshot" gorm:"type:text;not null"`
	DurationDays        int64  `json:"durationDays,string" gorm:"not null"`
	GrantedDuration     int64  `json:"grantedDuration,string" gorm:"not null"`
	CompensatedDuration int64  `json:"compensatedDuration,string" gorm:"not null;check:ck_guard_grant_duration,granted_duration > 0 AND compensated_duration >= 0 AND compensated_duration <= granted_duration"`
	Status              string `json:"status" gorm:"size:16;not null"`
	Sequence            int64  `json:"-" gorm:"not null"`
	EffectiveAt         int64  `json:"effectiveAt" gorm:"not null"`
	ExpiresAt           int64  `json:"expiresAt" gorm:"not null"`
	CreatedAt           int64  `json:"createdAt" gorm:"not null"`
	UpdatedAt           int64  `json:"updatedAt" gorm:"not null;autoUpdateTime:false"`
}

func (Grant) TableName() string { return "guard_grant" }

type Compensation struct {
	SourceSnapshot string `json:"sourceSnapshot" gorm:"type:text;not null"`
	ID             uint64 `json:"-" gorm:"primaryKey"`
	CompensationNo string `json:"compensationNo" gorm:"size:40;not null;uniqueIndex"`
	GrantNo        string `json:"grantNo" gorm:"size:40;not null;index"`
	MembershipNo   string `json:"membershipNo" gorm:"size:40;not null;index"`
	SourceType     string `json:"sourceType" gorm:"size:32;not null;uniqueIndex:uk_guard_comp_source,priority:1"`
	SourceNo       string `json:"sourceNo" gorm:"size:40;not null;uniqueIndex:uk_guard_comp_source,priority:2"`
	SourceEntryNo  string `json:"sourceEntryNo" gorm:"size:40;not null;uniqueIndex:uk_guard_comp_source,priority:3"`
	Duration       int64  `json:"duration,string" gorm:"not null;check:ck_guard_comp_duration,duration > 0"`
	Reason         string `json:"reason" gorm:"size:200;not null"`
	Status         string `json:"status" gorm:"size:16;not null"`
	Sequence       int64  `json:"-" gorm:"not null"`
	CreatedAt      int64  `json:"createdAt" gorm:"not null"`
	CompletedAt    int64  `json:"completedAt" gorm:"not null"`
}

func (Compensation) TableName() string { return "guard_compensation" }

type Ledger struct {
	ID             uint64 `json:"-" gorm:"primaryKey"`
	LedgerNo       string `json:"ledgerNo" gorm:"size:40;not null;uniqueIndex"`
	MembershipNo   string `json:"membershipNo" gorm:"size:40;not null;index"`
	EntryType      string `json:"entryType" gorm:"size:16;not null"`
	SourceNo       string `json:"sourceNo" gorm:"size:64;not null;uniqueIndex"`
	PlanNo         string `json:"planNo" gorm:"size:40;not null"`
	DurationDelta  int64  `json:"durationDelta,string" gorm:"not null"`
	BeforeSnapshot string `json:"beforeSnapshot" gorm:"type:longtext;not null"`
	AfterSnapshot  string `json:"afterSnapshot" gorm:"type:longtext;not null"`
	CreatedAt      int64  `json:"createdAt" gorm:"not null"`
}

func (Ledger) TableName() string { return "guard_ledger" }

type Task struct {
	ID          uint64 `json:"-" gorm:"primaryKey"`
	TaskNo      string `json:"taskNo" gorm:"size:40;not null;uniqueIndex"`
	Kind        string `json:"kind" gorm:"size:16;not null"`
	SourceNo    string `json:"sourceNo" gorm:"size:40;not null;uniqueIndex"`
	Status      string `json:"status" gorm:"size:16;not null;index:idx_guard_task_due,priority:1"`
	LeaseToken  string `json:"-" gorm:"size:40;not null"`
	LeaseUntil  int64  `json:"leaseUntil" gorm:"not null"`
	RetryCount  int    `json:"retryCount" gorm:"not null"`
	LastError   string `json:"lastError" gorm:"size:200;not null"`
	NextRetryAt int64  `json:"nextRetryAt" gorm:"not null;index:idx_guard_task_due,priority:2"`
	CompletedAt int64  `json:"completedAt" gorm:"not null"`
	CreatedAt   int64  `json:"createdAt" gorm:"not null"`
	UpdatedAt   int64  `json:"updatedAt" gorm:"not null;autoUpdateTime:false"`
}

func (Task) TableName() string { return "guard_task" }

type AdminPermission struct {
	ID          uint64 `json:"-" gorm:"primaryKey"`
	AuthorityID uint   `json:"-" gorm:"not null;uniqueIndex:uk_guard_permission,priority:1"`
	Path        string `json:"path" gorm:"size:100;not null;uniqueIndex:uk_guard_permission,priority:2"`
	Method      string `json:"method" gorm:"size:10;not null;uniqueIndex:uk_guard_permission,priority:3"`
}

func (AdminPermission) TableName() string { return "guard_admin_permission" }

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

func (Audit) TableName() string { return "guard_audit_log" }
