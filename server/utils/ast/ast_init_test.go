package ast

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"tb_live_module/global"
	"testing"
)

// 注入/回滚测试必须运行在临时副本，不能改写正在被其他包编译的仓库源文件。
func TestMain(m *testing.M) { os.Exit(runASTTests(m)) }
func runASTTests(m *testing.M) int {
	root, err := os.MkdirTemp("", "gva-ast-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(root)
	sourceRoot, err := filepath.Abs("../..")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	files := map[string]string{}
	for _, name := range []string{"initialize/gorm_biz.go", "initialize/router_biz.go", "api/v1/enter.go", "router/enter.go", "service/enter.go", "api/v1/example/enter.go", "router/example/enter.go", "service/example/enter.go", "plugin/register.go"} {
		files[name] = name
	}
	for _, name := range []string{"plugin.go", "api/enter.go", "router/enter.go", "service/enter.go", "initialize/gorm.go", "initialize/router.go"} {
		files["plugin/gva/"+name] = "plugin/announcement/" + name
	}
	files["plugin/gva/gen/main.go"] = "plugin/announcement/gen/gen.go"
	for target, source := range files {
		body, e := os.ReadFile(filepath.Join(sourceRoot, source))
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 1
		}
		if strings.HasPrefix(target, "plugin/gva/") {
			body = []byte(strings.ReplaceAll(string(body), "announcement", "gva"))
		}
		target = filepath.Join(root, "server", target)
		if e = os.MkdirAll(filepath.Dir(target), 0755); e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 1
		}
		if e = os.WriteFile(target, body, 0600); e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 1
		}
	}
	global.GVA_CONFIG.AutoCode.Root = root
	global.GVA_CONFIG.AutoCode.Server = "server"
	return m.Run()
}
