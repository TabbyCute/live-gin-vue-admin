package eventdelivery

// Event is an immutable, allowlisted notification projection, never a financial command.
// Times are UTC Unix milliseconds. No soft deletion: IDs remain bound permanently.
type Event struct {
	EventID       string `json:"eventId" gorm:"size:72;primaryKey"`
	Source        string `json:"source" gorm:"size:16;not null;uniqueIndex:uk_delivery_source,priority:1"`
	SourceID      string `json:"sourceId" gorm:"size:128;not null;uniqueIndex:uk_delivery_source,priority:2"`
	EventType     string `json:"eventType" gorm:"size:48;not null"`
	SchemaVersion uint8  `json:"schemaVersion" gorm:"not null"`
	Audience      string `json:"audience" gorm:"size:16;not null"`
	Destination   string `json:"-" gorm:"size:64;not null"`
	Payload       string `json:"-" gorm:"type:text;not null"`
	PayloadHash   string `json:"payloadHash" gorm:"size:64;not null"`
	OccurredAt    int64  `json:"occurredAt" gorm:"not null"`
	CreatedAt     int64  `json:"createdAt" gorm:"not null;autoCreateTime:false"`
}

func (Event) TableName() string { return "event_delivery_event" }

type Delivery struct {
	ID              uint64 `json:"id,string" gorm:"primaryKey"`
	EventID         string `json:"eventId" gorm:"size:72;not null;uniqueIndex:uk_delivery_consumer,priority:1"`
	Consumer        string `json:"consumer" gorm:"size:48;not null;uniqueIndex:uk_delivery_consumer,priority:2;index:idx_delivery_due,priority:1"`
	Status          string `json:"status" gorm:"size:16;not null;index:idx_delivery_due,priority:2;index:idx_delivery_admin,priority:1;check:ck_delivery_status,status IN ('pending','leased','blocked','delivered','dead')"`
	Attempts        uint32 `json:"attempts" gorm:"not null"`
	CycleAttempts   uint32 `json:"cycleAttempts" gorm:"not null"`
	RetryGeneration uint32 `json:"retryGeneration" gorm:"not null"`
	NextAttemptAt   int64  `json:"nextAttemptAt" gorm:"not null;index:idx_delivery_due,priority:3"`
	LeaseToken      string `json:"-" gorm:"size:36;not null"`
	LeaseUntil      int64  `json:"leaseUntil" gorm:"not null"`
	LastCode        string `json:"lastCode" gorm:"size:40;not null"`
	CreatedAt       int64  `json:"createdAt" gorm:"not null;autoCreateTime:false;index:idx_delivery_admin,priority:2"`
	UpdatedAt       int64  `json:"updatedAt" gorm:"not null;autoUpdateTime:false"`
	DeliveredAt     int64  `json:"deliveredAt" gorm:"not null"`
}

func (Delivery) TableName() string { return "event_delivery" }

// Attempt stores codes only; adapter errors/URLs/bodies/credentials are never persisted.
type Attempt struct {
	ID         uint64 `json:"id,string" gorm:"primaryKey"`
	DeliveryID uint64 `json:"deliveryId,string" gorm:"not null;uniqueIndex:uk_delivery_attempt,priority:1"`
	Number     uint32 `json:"number" gorm:"not null;uniqueIndex:uk_delivery_attempt,priority:2"`
	LeaseToken string `json:"-" gorm:"size:36;not null"`
	Outcome    string `json:"outcome" gorm:"size:40;not null"`
	StartedAt  int64  `json:"startedAt" gorm:"not null"`
	FinishedAt int64  `json:"finishedAt" gorm:"not null"`
}

func (Attempt) TableName() string { return "event_delivery_attempt" }

type RetryAudit struct {
	RequestID  string `json:"requestId" gorm:"size:64;primaryKey"`
	DeliveryID uint64 `json:"deliveryId,string" gorm:"not null;index"`
	ActorID    uint64 `json:"actorId,string" gorm:"not null"`
	Reason     string `json:"reason" gorm:"size:200;not null"`
	Generation uint32 `json:"generation" gorm:"not null"`
	CreatedAt  int64  `json:"createdAt" gorm:"not null;autoCreateTime:false"`
}

func (RetryAudit) TableName() string { return "event_delivery_retry_audit" }

// Cursor bounds repair work. It wraps after reaching the end so late commits with
// lower source IDs are eventually discovered; new writes also enqueue in their transaction.
type Cursor struct {
	Source    string `json:"source" gorm:"size:16;primaryKey"`
	LastKey   string `json:"-" gorm:"size:128;not null"`
	Scanned   uint64 `json:"scanned,string" gorm:"not null"`
	Cycles    uint64 `json:"cycles,string" gorm:"not null"`
	UpdatedAt int64  `json:"updatedAt" gorm:"not null;autoUpdateTime:false"`
}

func (Cursor) TableName() string { return "event_delivery_cursor" }
func Models() []any              { return []any{&Event{}, &Delivery{}, &Attempt{}, &RetryAudit{}, &Cursor{}} }
