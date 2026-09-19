package response

import "tb_live_module/config"

type SysConfigResponse struct {
	Config config.Server `json:"config"`
}
