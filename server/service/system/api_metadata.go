package system

import (
	"tb_live_module/global"
	"tb_live_module/model/system"
)

// removeAPIMetadata 删除代码生成器或插件卸载时产生的接口元数据。
// 这是内部维护能力，不再通过独立的 API 管理模块对外暴露。
func removeAPIMetadata(api system.SysApi) error {
	return removeAPIMetadataByIDs([]int{int(api.ID)})
}

func removeAPIMetadataByIDs(ids []int) error {
	if len(ids) == 0 {
		return nil
	}

	return global.GVA_DB.Delete(&system.SysApi{}, "id IN ?", ids).Error
}
