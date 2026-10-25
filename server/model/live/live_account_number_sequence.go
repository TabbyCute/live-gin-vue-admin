package live

const LiveAccountNumberStart uint64 = 1145781

// LiveAccountNumberSequence 保存客户端数字账号的下一个可分配值。
// 目前只使用ID=1的单行，并在注册事务中通过行锁串行分配账号编号。
type LiveAccountNumberSequence struct {
	ID        uint8  `json:"-" gorm:"column:id;type:tinyint unsigned;primaryKey"`
	NextValue uint64 `json:"-" gorm:"column:next_value;type:bigint unsigned;not null"`
}

func (LiveAccountNumberSequence) TableName() string {
	return "live_account_number_sequence"
}
