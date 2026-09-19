package initialize

import (
	_ "tb_live_module/source/example"
	_ "tb_live_module/source/system"
)

func init() {
	// do nothing,only import source package so that inits can be registered
}
