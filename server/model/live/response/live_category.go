package response

import "time"

type LiveCategoryPublicItem struct {
	ID       uint                     `json:"id"`
	ParentId uint                     `json:"parentId"`
	Code     string                   `json:"code"`
	Name     string                   `json:"name"`
	Icon     string                   `json:"icon"`
	Sort     int32                    `json:"sort"`
	Children []LiveCategoryPublicItem `json:"children"`
}

type LiveCategoryAdminItem struct {
	ID         uint      `json:"id"`
	ParentId   uint      `json:"parentId"`
	ParentName string    `json:"parentName"`
	Code       string    `json:"code"`
	Name       string    `json:"name"`
	Icon       string    `json:"icon"`
	Sort       int32     `json:"sort"`
	Status     uint8     `json:"status"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type LiveCategoryAdminTreeItem struct {
	ID       uint                        `json:"id"`
	ParentId uint                        `json:"parentId"`
	Code     string                      `json:"code"`
	Name     string                      `json:"name"`
	Icon     string                      `json:"icon"`
	Sort     int32                       `json:"sort"`
	Status   uint8                       `json:"status"`
	Children []LiveCategoryAdminTreeItem `json:"children"`
}

// LiveCategoryAdminListResp 与统一 PageResult JSON 结构一致，仅供 Swagger 标明列表元素类型。
type LiveCategoryAdminListResp struct {
	List     []LiveCategoryAdminItem `json:"list"`
	Total    int64                   `json:"total"`
	Page     int                     `json:"page"`
	PageSize int                     `json:"pageSize"`
}
