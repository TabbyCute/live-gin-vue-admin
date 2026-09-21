package system

type ServiceGroup struct {
	JwtService
	MenuService
	UserService
	InitDBService
	AutoCodeService
	BaseMenuService
	AuthorityService
	DictionaryService
	SystemConfigService
	OperationRecordService
	DictionaryDetailService
	SysExportTemplateService
	SysParamsService
	SysVersionService
	SkillsService
	AIWorkflowSession aiWorkflowSession
	AutoCodePlugin    autoCodePlugin
	AutoCodePackage   autoCodePackage
	AutoCodeHistory   autoCodeHistory
	AutoCodeTemplate  autoCodeTemplate
	SysErrorService
	LoginLogService
	ApiTokenService
}
