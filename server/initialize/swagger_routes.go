package initialize

import (
	"encoding/json"
	"github.com/swaggo/swag"
	"strings"
	"sync"
	"tb_live_module/docs"
)

var swaggerRoutesOnce sync.Once

const projectSwaggerInstance = "project-api"

type projectSwaggerDocument struct{}

func (projectSwaggerDocument) ReadDoc() string {
	return normalizeSwaggerPaths(docs.SwaggerInfo.ReadDoc())
}

// 旧管理注释使用/live/room等相对路径，新APP注释已经包含/v1/app。
// 统一以system.router-prefix为BasePath，补齐旧管理前缀，避免Swagger试调用误拼/admin/v1/app。
func normalizeSwaggerPaths(raw string) string {
	var doc map[string]interface{}
	if json.Unmarshal([]byte(raw), &doc) != nil {
		return raw
	}
	paths, ok := doc["paths"].(map[string]interface{})
	if !ok {
		return raw
	}
	normalized := make(map[string]interface{}, len(paths))
	for path, value := range paths {
		if !strings.HasPrefix(path, "/v1/") {
			path = adminV1RoutePrefix + path
		}
		normalized[path] = value
	}
	doc["paths"] = normalized
	encoded, err := json.Marshal(doc)
	if err != nil {
		return raw
	}
	return string(encoded)
}
func registerProjectSwagger() {
	swaggerRoutesOnce.Do(func() { swag.Register(projectSwaggerInstance, projectSwaggerDocument{}) })
}
