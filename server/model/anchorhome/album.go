package anchorhome

const (
	PhotoPending  = "pending"
	PhotoApproved = "approved"
	PhotoRejected = "rejected"
	PhotoRemoved  = "removed"
	PhotoDeleted  = "deleted"
)

// Album serializes owner and moderator writes; versions stay within JSON's safe integer range.
type Album struct {
	OwnerID uint64 `gorm:"column:owner_id;type:bigint unsigned;primaryKey;autoIncrement:false;check:ck_anchor_home_owner,owner_id > 0"`
	Version uint64 `gorm:"type:bigint unsigned;not null;default:0;check:ck_anchor_home_album_version,version <= 9007199254740991"`
}

func (Album) TableName() string { return "anchor_home_album" }

// Photo stores only sanitized, internally generated files. StorageKey must never be returned to clients.
type Photo struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement"`
	PhotoNo    string `gorm:"type:varchar(40);not null;uniqueIndex:uk_anchor_home_photo_no"`
	OwnerID    uint64 `gorm:"type:bigint unsigned;not null;index:idx_anchor_home_owner_status,priority:1;index:idx_anchor_home_upload_rate,priority:1"`
	AnchorNo   string `gorm:"type:varchar(32);not null;index:idx_anchor_home_anchor"`
	StorageKey string `gorm:"type:char(32);not null;uniqueIndex:uk_anchor_home_storage"`
	Status     string `gorm:"type:varchar(16);not null;index:idx_anchor_home_owner_status,priority:2;index:idx_anchor_home_moderation,priority:1;check:ck_anchor_home_photo_status,status IN ('pending','approved','rejected','removed','deleted')"`
	Version    uint64 `gorm:"type:bigint unsigned;not null;check:ck_anchor_home_photo_version,version > 0 AND version <= 9007199254740991"`
	Sort       int    `gorm:"not null;index:idx_anchor_home_owner_status,priority:3"`
	Width      int    `gorm:"not null;check:ck_anchor_home_photo_dimensions,width > 0 AND height > 0 AND width <= 2048 AND height <= 2048 AND bytes > 0 AND bytes <= 16777216 AND sort > 0 AND sort <= 2147483647"`
	Height     int    `gorm:"not null"`
	Bytes      int64  `gorm:"type:bigint;not null"`
	Reason     string `gorm:"type:varchar(800);not null;default:''"`
	CreatedAt  int64  `gorm:"type:bigint;not null;index:idx_anchor_home_moderation,priority:2;index:idx_anchor_home_upload_rate,priority:2"`
	UpdatedAt  int64  `gorm:"type:bigint;not null"`
}

func (Photo) TableName() string { return "anchor_home_photo" }

// Command stores the original successful result permanently, including after owner deletion.
type Command struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement"`
	ActorKind   string `gorm:"type:varchar(8);not null;uniqueIndex:uk_anchor_home_command,priority:1"`
	ActorID     uint64 `gorm:"type:bigint unsigned;not null;uniqueIndex:uk_anchor_home_command,priority:2"`
	RequestID   string `gorm:"type:varchar(64);not null;uniqueIndex:uk_anchor_home_command,priority:3"`
	PayloadHash string `gorm:"type:char(64);not null"`
	Result      string `gorm:"type:text;not null"`
	CreatedAt   int64  `gorm:"type:bigint;not null"`
}

func (Command) TableName() string { return "anchor_home_command" }

// Audit is append-only, protected by database triggers; it contains no user media or credentials.
type Audit struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	PhotoNo      string `gorm:"type:varchar(40);not null;index:idx_anchor_home_audit_photo"`
	OwnerID      uint64 `gorm:"type:bigint unsigned;not null"`
	ActorKind    string `gorm:"type:varchar(8);not null"`
	ActorID      uint64 `gorm:"type:bigint unsigned;not null"`
	RequestID    string `gorm:"type:varchar(64);not null"`
	Action       string `gorm:"type:varchar(16);not null"`
	FromStatus   string `gorm:"type:varchar(16);not null"`
	ToStatus     string `gorm:"type:varchar(16);not null"`
	Reason       string `gorm:"type:varchar(800);not null;default:''"`
	AlbumVersion uint64 `gorm:"type:bigint unsigned;not null"`
	CreatedAt    int64  `gorm:"type:bigint;not null"`
}

func (Audit) TableName() string { return "anchor_home_audit" }
