package request

import model "tb_live_module/model/social"

type Target struct {
	AccountNo string `json:"accountNo" form:"accountNo" binding:"omitempty,max=32" example:"10000001"`
	AnchorNo  string `json:"anchorNo" form:"anchorNo" binding:"omitempty,max=32" example:"100001"`
}
type TargetBatch struct {
	Targets []Target `json:"targets" binding:"required,min=1,max=100,dive"`
}
type Mute struct {
	Target
	Scope model.MuteScope `json:"scope" binding:"required,oneof=content live_notification recommendation direct_message" example:"content"`
}
type Resource struct {
	ResourceType model.ResourceType `json:"resourceType" form:"resourceType" binding:"required,oneof=live_room live_replay short_video" example:"live_room"`
	ResourceID   string             `json:"resourceId" form:"resourceId" binding:"required,max=64" example:"100001"`
}
type ResourceBatch struct {
	Resources []Resource `json:"resources" binding:"required,min=1,max=100,dive"`
}

// CursorSearch 是新社交接口的有界游标契约；不改动旧 PageInfo 接口。
type CursorSearch struct {
	Cursor       string             `json:"cursor" form:"cursor" binding:"omitempty,max=128"`
	Limit        int                `json:"limit" form:"limit" binding:"omitempty,min=1,max=100" example:"20"`
	ResourceType model.ResourceType `json:"resourceType" form:"resourceType" binding:"omitempty,oneof=live_room live_replay short_video"`
}
type CommonSearch struct {
	CursorSearch
	Target
}
type HistoryOpen struct {
	Resource
	RequestID string `json:"requestId" binding:"required,min=8,max=64" example:"watch-20261025-0001"`
}
type HistoryReport struct {
	Resource
	SessionToken   string `json:"sessionToken" binding:"required,uuid"`
	SessionVersion uint64 `json:"sessionVersion" binding:"required,lte=9007199254740991"`
	Epoch          uint64 `json:"epoch" binding:"lte=9007199254740991"`
	Sequence       uint64 `json:"sequence" binding:"required,lte=9007199254740991" example:"1"`
	PositionMs     int64  `json:"positionMs" binding:"gte=0,lte=604800000" example:"10000"`
	Completed      bool   `json:"completed"`
	Checkpoint     bool   `json:"checkpoint"` // true: 成功响应意味着 MySQL 提交；false: 有界可丢失心跳。
}
type CounterRepair struct {
	Target
	Confirm string `json:"confirm" binding:"required,eq=REPAIR"`
	Reason  string `json:"reason" binding:"required,min=4,max=200"`
}
type EventReplay struct {
	EventID string `json:"eventId" binding:"required,uuid"`
	Confirm string `json:"confirm" binding:"required,eq=REPLAY"`
	Reason  string `json:"reason" binding:"required,min=4,max=200"`
}
