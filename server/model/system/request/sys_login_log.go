package request

import (
	"tb_live_module/model/common/request"
	"tb_live_module/model/system"
)

type SysLoginLogSearch struct {
	system.SysLoginLog
	request.PageInfo
}
