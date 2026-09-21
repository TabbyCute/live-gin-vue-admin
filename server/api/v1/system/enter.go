package system

import "tb_live_module/service"

type ApiGroup struct {
	DBApi
	JwtApi
	BaseApi
	SystemApi
	AutoCodeApi
	AuthorityApi
	DictionaryApi
	AuthorityMenuApi
	OperationRecordApi
	DictionaryDetailApi
	SysExportTemplateApi
	AutoCodePluginApi
	AutoCodePackageApi
	AutoCodeHistoryApi
	AutoCodeTemplateApi
	SysParamsApi
	SysVersionApi
	SysErrorApi
	LoginLogApi
	ApiTokenApi
	SkillsApi
	AIWorkflowSessionApi
}

var (
	jwtService               = service.ServiceGroupApp.SystemServiceGroup.JwtService
	menuService              = service.ServiceGroupApp.SystemServiceGroup.MenuService
	userService              = service.ServiceGroupApp.SystemServiceGroup.UserService
	initDBService            = service.ServiceGroupApp.SystemServiceGroup.InitDBService
	baseMenuService          = service.ServiceGroupApp.SystemServiceGroup.BaseMenuService
	authorityService         = service.ServiceGroupApp.SystemServiceGroup.AuthorityService
	dictionaryService        = service.ServiceGroupApp.SystemServiceGroup.DictionaryService
	systemConfigService      = service.ServiceGroupApp.SystemServiceGroup.SystemConfigService
	sysParamsService         = service.ServiceGroupApp.SystemServiceGroup.SysParamsService
	operationRecordService   = service.ServiceGroupApp.SystemServiceGroup.OperationRecordService
	dictionaryDetailService  = service.ServiceGroupApp.SystemServiceGroup.DictionaryDetailService
	autoCodeService          = service.ServiceGroupApp.SystemServiceGroup.AutoCodeService
	aiWorkflowSessionService = service.ServiceGroupApp.SystemServiceGroup.AIWorkflowSession
	autoCodePluginService    = service.ServiceGroupApp.SystemServiceGroup.AutoCodePlugin
	autoCodePackageService   = service.ServiceGroupApp.SystemServiceGroup.AutoCodePackage
	autoCodeHistoryService   = service.ServiceGroupApp.SystemServiceGroup.AutoCodeHistory
	autoCodeTemplateService  = service.ServiceGroupApp.SystemServiceGroup.AutoCodeTemplate
	sysVersionService        = service.ServiceGroupApp.SystemServiceGroup.SysVersionService
	sysErrorService          = service.ServiceGroupApp.SystemServiceGroup.SysErrorService
	loginLogService          = service.ServiceGroupApp.SystemServiceGroup.LoginLogService
	apiTokenService          = service.ServiceGroupApp.SystemServiceGroup.ApiTokenService
	skillsService            = service.ServiceGroupApp.SystemServiceGroup.SkillsService
)
