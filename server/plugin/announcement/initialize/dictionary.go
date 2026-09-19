package initialize

import (
	"context"
	model "tb_live_module/model/system"
	"tb_live_module/plugin/plugin-tool/utils"
)

func Dictionary(ctx context.Context) {
	entities := []model.SysDictionary{}
	utils.RegisterDictionaries(entities...)
}
