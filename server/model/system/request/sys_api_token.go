package request

import (
	"tb_live_module/model/common/request"
	"tb_live_module/model/system"
)

type SysApiTokenSearch struct {
	system.SysApiToken
	request.PageInfo
    Status *bool `json:"status" form:"status"`
}
