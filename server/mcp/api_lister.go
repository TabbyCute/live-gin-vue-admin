package mcpTool

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/mcp"
)

func init() {
	RegisterTool(&ApiLister{})
}

type ApiInfo struct {
	ID          uint   `json:"id,omitempty"`
	Path        string `json:"path"`
	Description string `json:"description,omitempty"`
	ApiGroup    string `json:"apiGroup,omitempty"`
	Method      string `json:"method"`
	Source      string `json:"source"`
}

type ApiListResponse struct {
	Success    bool      `json:"success"`
	Message    string    `json:"message"`
	GinApis    []ApiInfo `json:"ginApis"`
	TotalCount int       `json:"totalCount"`
}

type mcpRoutesResponse struct {
	Routes gin.RoutesInfo `json:"routes"`
}

type ApiLister struct{}

func (a *ApiLister) New() mcp.Tool {
	return mcp.NewTool("list_all_apis",
		mcp.WithDescription(`获取 Gin 中实际注册的 API 路由：

**功能说明：**
- 返回gin框架中实际注册的路由API列表
- 帮助前端判断是使用现有API还是需要创建新的API,如果api在前端未使用且需要前端调用的时候，请到api文件夹下对应模块的js中添加方法并暴露给当前业务调用

**返回数据结构：**
- ginApis: gin路由中的API（仅包含路径和方法），需要AI根据路径自行揣摩路径的业务含义，例如：/api/user/:id 表示根据用户ID获取用户信息`),
		mcp.WithString("_placeholder",
			mcp.Description("占位符，防止json schema校验失败"),
		),
	)
}

func (a *ApiLister) Handle(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	routeResp, err := postUpstream[mcpRoutesResponse](ctx, "/autoCode/mcpRoutes", map[string]any{})
	if err != nil {
		return nil, err
	}

	ginApis := make([]ApiInfo, 0, len(routeResp.Data.Routes))
	for _, route := range routeResp.Data.Routes {
		ginApis = append(ginApis, ApiInfo{
			Path:   route.Path,
			Method: route.Method,
			Source: "gin",
		})
	}

	return textResultWithJSON("", ApiListResponse{
		Success:    true,
		Message:    "获取API列表成功",
		GinApis:    ginApis,
		TotalCount: len(ginApis),
	})
}
