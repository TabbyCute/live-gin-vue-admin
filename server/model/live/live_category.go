package live

import "tb_live_module/global"

const (
	LiveCategoryStatusDisabled uint8 = iota
	LiveCategoryStatusEnabled
)

// LiveCategory 是直播业务的统一分类来源。
// 分类 ID 会作为公开的 categoryId 返回客户端，不属于需要隐藏的业务主键。
type LiveCategory struct {
	global.GVA_MODEL
	ParentId uint   `json:"parentId" gorm:"column:parent_id;type:bigint unsigned;not null;default:0;index:idx_live_category_tree,priority:1;comment:父分类ID，0表示顶级分类"`
	Code     string `json:"code" gorm:"column:code;type:varchar(32);not null;uniqueIndex:uk_live_category_code;comment:稳定分类编码，创建后不可修改"`
	Name     string `json:"name" gorm:"column:name;type:varchar(64);not null;default:'';comment:分类名称"`
	Icon     string `json:"icon" gorm:"column:icon;type:varchar(500);not null;default:'';comment:分类图标URL"`
	Sort     int32  `json:"sort" gorm:"column:sort;type:int;not null;default:0;index:idx_live_category_tree,priority:3,sort:desc;comment:排序权重，数值越大越靠前"`
	Status   uint8  `json:"status" gorm:"column:status;type:tinyint unsigned;not null;default:1;index:idx_live_category_tree,priority:2;comment:分类状态：0停用 1启用"`
}

func (LiveCategory) TableName() string {
	return "live_category"
}
