package system

import (
	"context"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"tb_live_module/config"
	"tb_live_module/global"
	"testing"

	model "tb_live_module/model/system"
	"tb_live_module/model/system/request"
)

func Test_autoCodePackage_Create(t *testing.T) {
	setupAutoCodeFixture(t)
	type args struct {
		ctx  context.Context
		info *request.SysAutoCodePackageCreate
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "测试 package",
			args: args{
				ctx: context.Background(),
				info: &request.SysAutoCodePackageCreate{
					Template:    "package",
					PackageName: "gva",
				},
			},
			wantErr: false,
		},
		{
			name: "测试 plugin",
			args: args{
				ctx: context.Background(),
				info: &request.SysAutoCodePackageCreate{
					Template:    "plugin",
					PackageName: "gva",
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &autoCodePackage{}
			if err := a.Create(tt.args.ctx, tt.args.info); (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_autoCodePackage_templates(t *testing.T) {
	setupAutoCodeFixture(t)
	type args struct {
		ctx       context.Context
		entity    model.SysAutoCodePackage
		info      request.AutoCode
		isPackage bool
	}
	tests := []struct {
		name      string
		args      args
		wantCode  map[string]string
		wantEnter map[string]map[string]string
		wantErr   bool
	}{
		{
			name: "测试1",
			args: args{
				ctx: context.Background(),
				entity: model.SysAutoCodePackage{
					Desc:        "描述",
					Label:       "展示名",
					Template:    "plugin",
					PackageName: "preview",
				},
				info: request.AutoCode{
					Abbreviation:    "user",
					HumpPackageName: "user",
				},
				isPackage: false,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &autoCodePackage{}
			gotCode, gotEnter, gotCreates, err := s.templates(tt.args.ctx, tt.args.entity, tt.args.info, tt.args.isPackage)
			if (err != nil) != tt.wantErr {
				t.Errorf("templates() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			for key, value := range gotCode {
				t.Logf("\n")
				t.Log(key)
				t.Log(value)
				t.Logf("\n")
			}
			t.Log(gotCreates)
			if len(gotEnter) != len(tt.wantEnter) {
				t.Errorf("templates() gotEnter = %v, want %v", gotEnter, tt.wantEnter)
			}
		})
	}
}

// 代码生成测试必须有隔离数据库与输出目录，不依赖外部开发环境或改写项目入口。
func setupAutoCodeFixture(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	serverRoot, err := filepath.Abs("../..")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "server"), 0755))
	require.NoError(t, os.CopyFS(filepath.Join(root, "server", "resource"), os.DirFS(filepath.Join(serverRoot, "resource"))))
	for _, name := range []string{"api/v1/enter.go", "router/enter.go", "service/enter.go", "plugin/register.go"} {
		b, e := os.ReadFile(filepath.Join(serverRoot, name))
		require.NoError(t, e)
		target := filepath.Join(root, "server", name)
		require.NoError(t, os.MkdirAll(filepath.Dir(target), 0755))
		require.NoError(t, os.WriteFile(target, b, 0600))
	}
	db, err := gorm.Open(sqlite.Open(filepath.Join(root, "test.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.SysAutoCodePackage{}))
	oldDB, oldConfig := global.GVA_DB, global.GVA_CONFIG.AutoCode
	global.GVA_DB = db
	global.GVA_CONFIG.AutoCode = config.Autocode{Root: root, Server: "server", Web: "web", Module: "tb_live_module"}
	t.Cleanup(func() {
		global.GVA_DB = oldDB
		global.GVA_CONFIG.AutoCode = oldConfig
		pool, _ := db.DB()
		pool.Close()
	})
}
