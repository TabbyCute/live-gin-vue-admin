package postgresql_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	adapter "github.com/casbin/gorm-adapter/v3"
	"github.com/flipped-aurora/gin-vue-admin/server/config"
	"github.com/flipped-aurora/gin-vue-admin/server/global"
	"github.com/flipped-aurora/gin-vue-admin/server/initialize"
	"github.com/flipped-aurora/gin-vue-admin/server/model/example"
	"github.com/flipped-aurora/gin-vue-admin/server/model/system"
	systemReq "github.com/flipped-aurora/gin-vue-admin/server/model/system/request"
	announcement "github.com/flipped-aurora/gin-vue-admin/server/plugin/announcement/model"
	autoModel "github.com/flipped-aurora/gin-vue-admin/server/plugin/auto/model"
	"github.com/flipped-aurora/gin-vue-admin/server/utils"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// Run only against a newly created disposable database named codex_init_audit_*.
func TestInitializationSnapshot(t *testing.T) {
	dsn := os.Getenv("GVA_INIT_TEST_DSN")
	if dsn == "" {
		t.Skip("set GVA_INIT_TEST_DSN to an empty disposable PostgreSQL database")
	}
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("cannot connect to audit database")
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	defer sqlDB.Close()
	var name string
	if err := db.Raw("SELECT current_database()").Scan(&name).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(name, "codex_init_audit_") {
		t.Fatal("refusing to initialize a non-audit database")
	}
	count := func(query string) int64 {
		t.Helper()
		var n int64
		if err := db.Raw(query).Scan(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	tableCount := "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'"
	if count(tableCount) != 0 {
		t.Fatal("audit database must be empty")
	}
	script, err := os.ReadFile("init.sql")
	if err != nil {
		t.Fatal(err)
	}
	// Inject a late failure to prove that tables AND seed records roll back.
	broken := strings.Replace(string(script), "COMMIT;", "SELECT 1/0;\nCOMMIT;", 1)
	if err := db.Exec(broken).Error; err == nil {
		t.Fatal("injected failure did not abort")
	}
	db.Exec("ROLLBACK")
	if count(tableCount) != 0 {
		t.Fatal("failed initialization left partial tables")
	}
	if err := db.Exec(string(script)).Error; err != nil {
		t.Fatal(err)
	}
	if count(tableCount) != 33 {
		t.Fatal("unexpected table count")
	}
	if err := db.Exec(string(script)).Error; err == nil {
		t.Fatal("non-empty target was accepted")
	}
	db.Exec("ROLLBACK")
	if count("SELECT count(*) FROM sys_users") != 1 {
		t.Fatal("unexpected seeded accounts")
	}
	if count("SELECT count(*) FROM sys_authorities") != 1 {
		t.Fatal("demo roles remain in initialization")
	}
	var admin system.SysUser
	if err := db.Where("username = ?", "admin").First(&admin).Error; err != nil {
		t.Fatal(err)
	}
	if !utils.BcryptCheck("888888", admin.Password) || admin.AuthorityId != 888 || admin.Enable != 1 {
		t.Fatal("default administrator credentials or authority are incorrect")
	}
	checks := map[string]string{
		"missing admin APIs":             "SELECT count(*) FROM sys_apis a WHERE deleted_at IS NULL AND NOT EXISTS (SELECT 1 FROM casbin_rule p WHERE p.ptype='p' AND p.v0='888' AND p.v1=a.path AND p.v2=a.method)",
		"missing admin menus":            "SELECT count(*) FROM sys_base_menus m WHERE deleted_at IS NULL AND NOT EXISTS (SELECT 1 FROM sys_authority_menus a WHERE a.sys_authority_authority_id=888 AND a.sys_base_menu_id=m.id)",
		"orphan menu grants":             "SELECT count(*) FROM sys_authority_menus a LEFT JOIN sys_base_menus m ON a.sys_base_menu_id=m.id LEFT JOIN sys_authorities r ON a.sys_authority_authority_id=r.authority_id WHERE m.id IS NULL OR r.authority_id IS NULL",
		"orphan user roles":              "SELECT count(*) FROM sys_user_authority a LEFT JOIN sys_users u ON a.sys_user_id=u.id LEFT JOIN sys_authorities r ON a.sys_authority_authority_id=r.authority_id WHERE u.id IS NULL OR r.authority_id IS NULL",
		"orphan menu parents":            "SELECT count(*) FROM sys_base_menus m LEFT JOIN sys_base_menus p ON m.parent_id=p.id WHERE m.parent_id<>0 AND p.id IS NULL",
		"deleted menu references":        "SELECT count(*) FROM sys_base_menus WHERE name='about' OR component LIKE 'view/about/%' OR path IN ('https://www.gin-vue-admin.com/', 'https://www.gin-vue-admin.com')",
		"removed initialization APIs":    "SELECT count(*) FROM sys_apis WHERE path LIKE '/init/%'",
		"removed initialization ignores": "SELECT count(*) FROM sys_ignore_apis WHERE path LIKE '/init/%'",
		"duplicate API records":          "SELECT count(*) FROM (SELECT path,method FROM sys_apis GROUP BY path,method HAVING count(*)>1) a",
		"orphan policies":                "SELECT count(*) FROM casbin_rule p LEFT JOIN sys_authorities r ON p.v0=r.authority_id::text WHERE p.ptype='p' AND r.authority_id IS NULL",
	}
	for label, query := range checks {
		if n := count(query); n != 0 {
			t.Errorf("%s: %d", label, n)
		}
	}
	models := []interface{}{
		&adapter.CasbinRule{}, &system.SysApi{}, &system.SysIgnoreApi{}, &system.SysUser{}, &system.SysBaseMenu{},
		&system.JwtBlacklist{}, &system.SysAuthority{}, &system.SysDictionary{}, &system.SysDictionaryDetail{},
		&system.SysOperationRecord{}, &system.SysBaseMenuParameter{}, &system.SysBaseMenuBtn{}, &system.SysAuthorityBtn{},
		&system.SysAutoCodeHistory{}, &system.SysAutoCodePackage{}, &system.SysAIWorkflowSession{},
		&system.SysExportTemplate{}, &system.Condition{}, &system.JoinTemplate{}, &system.SysParams{},
		&system.SysVersion{}, &system.SysError{}, &system.SysApiToken{}, &system.SysLoginLog{},
		&example.ExaFile{}, &example.ExaCustomer{}, &example.ExaFileChunk{}, &example.ExaFileUploadAndDownload{},
		&example.ExaAttachmentCategory{}, &announcement.Info{}, &autoModel.SysAIWorkflowSession{},
		&autoModel.SysAutoCodeHistory{}, &autoModel.SysAutoCodePackage{},
	}
	seen := map[string]bool{}
	var checkSchema func(*schema.Schema)
	checkSchema = func(s *schema.Schema) {
		key := s.Table + ":" + s.Name
		if seen[key] {
			return
		}
		seen[key] = true
		var columns []string
		if err := db.Raw("SELECT column_name FROM information_schema.columns WHERE table_schema='public' AND table_name=?", s.Table).Scan(&columns).Error; err != nil {
			t.Fatal(err)
		}
		present := map[string]bool{}
		for _, column := range columns {
			present[column] = true
		}
		for _, field := range s.Fields {
			if field.DBName != "" && !field.IgnoreMigration && !present[field.DBName] {
				t.Errorf("missing model column: %s.%s", s.Table, field.DBName)
			}
		}
		for _, relation := range s.Relationships.Relations {
			if relation.JoinTable != nil {
				checkSchema(relation.JoinTable)
			}
		}
	}
	for _, model := range models {
		stmt := &gorm.Statement{DB: db}
		if err := stmt.Parse(model); err != nil {
			t.Fatal(err)
		}
		checkSchema(stmt.Schema)
	}
	var menus []system.SysBaseMenu
	if err := db.Find(&menus).Error; err != nil {
		t.Fatal(err)
	}
	for _, menu := range menus {
		if menu.Component != "" && !strings.HasPrefix(menu.Component, "http") {
			if _, err := os.Stat(filepath.Join("../../../web/src", menu.Component)); err != nil {
				t.Errorf("menu %s references missing component %s", menu.Name, menu.Component)
			}
		}
	}
	// Reject DDL while bootstrapping routes and Casbin to detect hidden AutoMigrate.
	if err := db.Exec(`CREATE FUNCTION public.audit_reject_ddl() RETURNS event_trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'unexpected startup DDL'; END $$;
        CREATE EVENT TRIGGER audit_reject_ddl ON ddl_command_start EXECUTE FUNCTION public.audit_reject_ddl();`).Error; err != nil {
		t.Fatal(err)
	}
	global.GVA_DB = db
	global.GVA_LOG = zap.NewNop()
	global.GVA_CONFIG = config.Server{}
	global.GVA_CONFIG.System.DisableAutoMigrate = true
	global.GVA_CONFIG.JWT.SigningKey = strings.Repeat("audit-only-key", 4)
	global.GVA_CONFIG.JWT.ExpiresTime = "7d"
	global.GVA_CONFIG.JWT.BufferTime = "1d"
	global.GVA_CONFIG.Local.StorePath = "/uploads/file"
	initialize.OtherInit()
	initialize.RegisterTables()
	gin.SetMode(gin.TestMode)
	router := initialize.Routers()
	enforcer := utils.GetCasbin()
	if enforcer == nil {
		t.Fatal("Casbin failed to load with DDL disabled")
	}
	public := map[string]bool{
		"/base/login": true, "/base/captcha": true, "/health": true, "/swagger/*any": true,
		"/uploads/file/*filepath": true, "/info/getInfoDataSource": true, "/info/getInfoPublic": true,
		"/sysError/createSysError": true, "/sysExportTemplate/exportExcelByToken": true,
		"/sysExportTemplate/exportTemplateByToken": true,
	}
	for _, route := range router.Routes() {
		if public[route.Path] {
			continue
		}
		allowed, err := enforcer.Enforce("888", route.Path, route.Method)
		if err != nil || !allowed {
			t.Errorf("registered route missing admin permission: %s %s", route.Method, route.Path)
		}
	}
	// Plugin startup must not add APIs/menus missing from the SQL snapshot.
	for _, label := range []string{"missing admin APIs", "missing admin menus", "duplicate API records"} {
		if n := count(checks[label]); n != 0 {
			t.Errorf("after startup %s: %d", label, n)
		}
	}
	protected := []string{"/autoCode/llmAuto", "/autoCode/llmAutoSSE", "/autoCode/initMenu", "/autoCode/initAPI", "/autoCode/initDictionary", "/skills/downloadOnlineSkill"}
	{
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/freshCasbin", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatal("anonymous policy refresh was allowed")
		}
	}
	jwt := utils.NewJWT()
	token, err := jwt.CreateToken(jwt.CreateClaims(systemReq.BaseClaims{ID: 1, AuthorityId: 999999}))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range protected {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}")))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("anonymous %s status=%d", path, rec.Code)
		}
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
		req.Header.Set("x-token", token)
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		var result struct {
			Code int
			Msg  string
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || result.Code != 7 || result.Msg != "权限不足" {
			t.Errorf("unprivileged %s not denied: %s", path, rec.Body.String())
		}
		allowed, err := enforcer.Enforce("888", path, "POST")
		if err != nil || !allowed {
			t.Errorf("admin missing %s permission", path)
		}
	}
	for _, path := range []string{"/init/initdb", "/init/checkdb"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("removed route %s returned %d", path, rec.Code)
		}
	}
	t.Run("existing_database_permission_patch", func(t *testing.T) {
		patch, err := os.ReadFile("security_permissions.sql")
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("DELETE FROM casbin_rule WHERE v0='888' AND v1='/autoCode/initMenu'").Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("DELETE FROM sys_apis WHERE path='/autoCode/initMenu'").Error; err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			if err := db.Exec(string(patch)).Error; err != nil {
				t.Fatal(err)
			}
		}
		if n := count(checks["missing admin APIs"]); n != 0 {
			t.Errorf("permission patch left %d missing grants", n)
		}
		if count(checks["duplicate API records"]) != 0 {
			t.Fatal("permission patch created duplicate APIs")
		}
		if n := count("SELECT count(*) FROM casbin_rule WHERE v0='888' AND v1='/autoCode/initMenu' AND v2='POST'"); n != 1 {
			t.Fatalf("expected one restored grant, got %d", n)
		}
	})
	t.Logf("verified %d tables, %d menus, bcrypt login, permissions, rollback, and startup without DDL", count(tableCount), len(menus))
}
