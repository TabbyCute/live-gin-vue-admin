package request

import commonReq "tb_live_module/model/common/request"

type LiveCategoryAdminListReq struct {
	commonReq.PageInfo
	Keyword  string `json:"keyword" form:"keyword" binding:"omitempty,max=64"`
	ParentId *uint  `json:"parentId" form:"parentId"`
	Status   *uint8 `json:"status" form:"status" binding:"omitempty,oneof=0 1"`
}

type LiveCategoryCreateReq struct {
	ParentId uint   `json:"parentId"`
	Code     string `json:"code" binding:"required,max=32" example:"music"`
	Name     string `json:"name" binding:"required,max=64" example:"音乐"`
	Icon     string `json:"icon" binding:"omitempty,max=500"`
	Sort     int32  `json:"sort"`
	Status   *uint8 `json:"status" binding:"required,oneof=0 1"`
}

// LiveCategoryUpdateReq 不允许修改 Code；编码是跨系统使用的稳定标识。
type LiveCategoryUpdateReq struct {
	ID       uint   `json:"id" binding:"required,gt=0"`
	ParentId uint   `json:"parentId"`
	Name     string `json:"name" binding:"required,max=64" example:"音乐"`
	Icon     string `json:"icon" binding:"omitempty,max=500"`
	Sort     int32  `json:"sort"`
}

type LiveCategoryStatusUpdateReq struct {
	ID     uint   `json:"id" binding:"required,gt=0"`
	Status *uint8 `json:"status" binding:"required,oneof=0 1"`
}

type LiveCategoryDeleteReq struct {
	ID uint `json:"id" binding:"required,gt=0"`
}
