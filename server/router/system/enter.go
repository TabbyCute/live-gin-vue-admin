package system

import api "tb_live_module/api/v1"

type RouterGroup struct {
	JwtRouter
	SysRouter
	BaseRouter
	InitRouter
	MenuRouter
	UserRouter
	AutoCodeRouter
	AuthorityRouter
	DictionaryRouter
	OperationRecordRouter
	DictionaryDetailRouter
	SysExportTemplateRouter
	SysParamsRouter
	SysVersionRouter
	SysErrorRouter
	LoginLogRouter
	ApiTokenRouter
	SkillsRouter
}

var (
	dbApi                = api.ApiGroupApp.SystemApiGroup.DBApi
	jwtApi               = api.ApiGroupApp.SystemApiGroup.JwtApi
	baseApi              = api.ApiGroupApp.SystemApiGroup.BaseApi
	systemApi            = api.ApiGroupApp.SystemApiGroup.SystemApi
	sysParamsApi         = api.ApiGroupApp.SystemApiGroup.SysParamsApi
	autoCodeApi          = api.ApiGroupApp.SystemApiGroup.AutoCodeApi
	authorityApi         = api.ApiGroupApp.SystemApiGroup.AuthorityApi
	dictionaryApi        = api.ApiGroupApp.SystemApiGroup.DictionaryApi
	authorityMenuApi     = api.ApiGroupApp.SystemApiGroup.AuthorityMenuApi
	autoCodePluginApi    = api.ApiGroupApp.SystemApiGroup.AutoCodePluginApi
	autocodeHistoryApi   = api.ApiGroupApp.SystemApiGroup.AutoCodeHistoryApi
	operationRecordApi   = api.ApiGroupApp.SystemApiGroup.OperationRecordApi
	autoCodePackageApi   = api.ApiGroupApp.SystemApiGroup.AutoCodePackageApi
	dictionaryDetailApi  = api.ApiGroupApp.SystemApiGroup.DictionaryDetailApi
	autoCodeTemplateApi  = api.ApiGroupApp.SystemApiGroup.AutoCodeTemplateApi
	exportTemplateApi    = api.ApiGroupApp.SystemApiGroup.SysExportTemplateApi
	sysVersionApi        = api.ApiGroupApp.SystemApiGroup.SysVersionApi
	sysErrorApi          = api.ApiGroupApp.SystemApiGroup.SysErrorApi
	skillsApi            = api.ApiGroupApp.SystemApiGroup.SkillsApi
	aiWorkflowSessionApi = api.ApiGroupApp.SystemApiGroup.AIWorkflowSessionApi
)
