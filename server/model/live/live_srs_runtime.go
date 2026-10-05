package live

const (
	LiveSRSHealthUnknown uint8 = iota
	LiveSRSHealthHealthy
	LiveSRSHealthDegraded
)

// LiveSRSRuntime 是单SRS部署的全局运行状态，只允许存在ID=1的一行。
// SRS故障只更新这一行，避免每个探测周期对一万条活动场次做无意义写放大。
type LiveSRSRuntime struct {
	ID            uint   `json:"id" gorm:"primaryKey;autoIncrement:false"`
	ServerID      string `json:"serverId" gorm:"column:server_id;type:varchar(128);not null;default:'';comment:SRS HTTP API返回的当前server标识"`
	ServiceID     string `json:"serviceId" gorm:"column:service_id;type:varchar(128);not null;default:'';comment:SRS HTTP API返回的当前service标识"`
	Generation    uint64 `json:"generation" gorm:"column:generation;type:bigint unsigned;not null;default:0;comment:检测到server ID变化时递增"`
	HealthState   uint8  `json:"healthState" gorm:"column:health_state;type:tinyint unsigned;not null;default:0;comment:0未知 1健康 2降级"`
	LastSuccessAt int64  `json:"lastSuccessAt" gorm:"column:last_success_at;type:bigint;not null;default:0"`
	LastFailureAt int64  `json:"lastFailureAt" gorm:"column:last_failure_at;type:bigint;not null;default:0"`
	DegradedAt    int64  `json:"degradedAt" gorm:"column:degraded_at;type:bigint;not null;default:0"`
	RecoveredAt   int64  `json:"recoveredAt" gorm:"column:recovered_at;type:bigint;not null;default:0"`
	LastError     string `json:"lastError" gorm:"column:last_error;type:varchar(500);not null;default:''"`
	Version       uint64 `json:"version" gorm:"column:version;type:bigint unsigned;not null;default:0;comment:全局状态CAS版本"`
}

func (LiveSRSRuntime) TableName() string { return "live_srs_runtime" }

// LiveSessionMediaAudit 保存高风险人工媒体确认的不可变业务审计记录。
type LiveSessionMediaAudit struct {
	ID                 uint64 `json:"id" gorm:"primaryKey;autoIncrement"`
	CreatedAt          int64  `json:"createdAt" gorm:"column:created_at;type:bigint;not null"`
	SessionID          uint   `json:"sessionId" gorm:"column:session_id;type:bigint unsigned;not null;index:idx_live_media_audit_session"`
	RoomID             uint   `json:"roomId" gorm:"column:room_id;type:bigint unsigned;not null"`
	OperatorID         uint   `json:"operatorId" gorm:"column:operator_id;type:bigint unsigned;not null"`
	OperatorName       string `json:"operatorName" gorm:"column:operator_name;type:varchar(128);not null;default:''"`
	Reason             string `json:"reason" gorm:"column:reason;type:varchar(500);not null"`
	Evidence           string `json:"evidence" gorm:"column:evidence;type:varchar(1000);not null"`
	ClientIP           string `json:"clientIp" gorm:"column:client_ip;type:varchar(64);not null;default:''"`
	UserAgent          string `json:"userAgent" gorm:"column:user_agent;type:varchar(500);not null;default:''"`
	RequestID          string `json:"requestId" gorm:"column:request_id;type:varchar(128);not null;default:''"`
	PublisherEpoch     uint64 `json:"publisherEpoch" gorm:"column:publisher_epoch;type:bigint unsigned;not null"`
	SRSGeneration      uint64 `json:"srsGeneration" gorm:"column:srs_generation;type:bigint unsigned;not null"`
	SRSHealthState     uint8  `json:"srsHealthState" gorm:"column:srs_health_state;type:tinyint unsigned;not null"`
	SRSLastSuccessAt   int64  `json:"srsLastSuccessAt" gorm:"column:srs_last_success_at;type:bigint;not null"`
	PreviousMediaState uint8  `json:"previousMediaState" gorm:"column:previous_media_state;type:tinyint unsigned;not null"`
}

func (LiveSessionMediaAudit) TableName() string { return "live_session_media_audit" }
