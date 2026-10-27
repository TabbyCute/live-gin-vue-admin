package social

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"tb_live_module/middleware"
	"tb_live_module/model/common/response"
	req "tb_live_module/model/social/request"
	res "tb_live_module/model/social/response"
	svc "tb_live_module/service/social"
)

type SocialApi struct{}

var _ = res.Change{}

// Guard 限制解析体积、DB/Redis时间和多实例请求频率；必须挂在JWT之后。
func Guard() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10)
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		actor, ok := middleware.GetAppAccountID(c)
		if !ok {
			response.NoAuth("未登录", c)
			c.Abort()
			return
		}
		read := c.Request.Method == http.MethodGet || strings.HasSuffix(c.FullPath(), "/batch-status") || strings.HasSuffix(c.FullPath(), "/progress/batch")
		workerRequired := true
		for _, suffix := range []string{"/block", "/unblock", "/unfollow", "/unmute", "/live/unsubscribe", "/unfavorite", "/history/delete", "/history/clear"} {
			if strings.HasSuffix(c.FullPath(), suffix) {
				workerRequired = false
				break
			}
		}
		if err := socialService.RateLimitRequest(ctx, uint64(actor), read, workerRequired); err != nil {
			respond(c, nil, err)
			c.Abort()
			return
		}
		c.Next()
	}
}
func actor(c *gin.Context) uint64 { id, _ := middleware.GetAppAccountID(c); return uint64(id) }
func bind(c *gin.Context, v interface{}) bool {
	var err error
	if c.Request.Method == http.MethodGet {
		err = c.ShouldBindQuery(v)
	} else {
		err = c.ShouldBindJSON(v)
	}
	if err != nil {
		respond(c, nil, svc.ErrInvalid)
		return false
	}
	return true
}
func respond(c *gin.Context, data interface{}, err error) {
	if err == nil {
		response.OkWithData(data, c)
		return
	}
	var b *svc.BusinessError
	if errors.As(err, &b) {
		response.Result(b.Code, struct{}{}, b.Message, c)
		return
	}
	response.Result(svc.ErrUnavailable.Code, struct{}{}, svc.ErrUnavailable.Message, c)
}
func change(c *gin.Context, action svc.Action) {
	var r req.Target
	if !bind(c, &r) {
		return
	}
	data, err := socialService.ChangeRelationship(c.Request.Context(), actor(c), r, action, "")
	respond(c, data, err)
}
func batch(c *gin.Context) {
	var r req.TargetBatch
	if !bind(c, &r) {
		return
	}
	data, err := socialService.BatchStatus(c.Request.Context(), actor(c), r.Targets)
	respond(c, data, err)
}
func userList(c *gin.Context, kind string) {
	var r req.CursorSearch
	if !bind(c, &r) {
		return
	}
	data, err := socialService.ListUsers(c.Request.Context(), actor(c), kind, r, 0)
	respond(c, data, err)
}

// Follow
// @Tags SocialApp
// @Summary 关注用户或主播
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.Target true "请求参数"
// @Success 200 {object} response.Response{data=res.Change}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/follow [post]
func (a *SocialApi) Follow(c *gin.Context) { change(c, svc.Follow) }

// Unfollow
// @Tags SocialApp
// @Summary 取消关注并关闭提醒
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.Target true "请求参数"
// @Success 200 {object} response.Response{data=res.Change}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/unfollow [post]
func (a *SocialApi) Unfollow(c *gin.Context) { change(c, svc.Unfollow) }

// Block
// @Tags SocialApp
// @Summary 拉黑并原子解除双向关注
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.Target true "请求参数"
// @Success 200 {object} response.Response{data=res.Change}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/block [post]
func (a *SocialApi) Block(c *gin.Context) { change(c, svc.Block) }

// Unblock
// @Tags SocialApp
// @Summary 取消拉黑，不恢复关注
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.Target true "请求参数"
// @Success 200 {object} response.Response{data=res.Change}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/unblock [post]
func (a *SocialApi) Unblock(c *gin.Context) { change(c, svc.Unblock) }

// Subscribe
// @Tags SocialApp
// @Summary 开启主播开播提醒
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.Target true "请求参数"
// @Success 200 {object} response.Response{data=res.Change}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/live/subscribe [post]
func (a *SocialApi) Subscribe(c *gin.Context) { change(c, svc.Subscribe) }

// Unsubscribe
// @Tags SocialApp
// @Summary 关闭主播开播提醒
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.Target true "请求参数"
// @Success 200 {object} response.Response{data=res.Change}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/live/unsubscribe [post]
func (a *SocialApi) Unsubscribe(c *gin.Context) { change(c, svc.Unsubscribe) }

// FollowBatch
// @Tags SocialApp
// @Summary 批量查询本人关系（最多100个目标，自动去重）
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.TargetBatch true "请求参数"
// @Success 200 {object} response.Response{data=[]res.Relationship}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/follow/batch-status [post]
func (a *SocialApi) FollowBatch(c *gin.Context) { batch(c) }

// BlockBatch
// @Tags SocialApp
// @Summary 批量查询本人关系（最多100个目标，自动去重）
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.TargetBatch true "请求参数"
// @Success 200 {object} response.Response{data=[]res.Relationship}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/block/batch-status [post]
func (a *SocialApi) BlockBatch(c *gin.Context) { batch(c) }

// MuteBatch
// @Tags SocialApp
// @Summary 批量查询本人关系（最多100个目标，自动去重）
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.TargetBatch true "请求参数"
// @Success 200 {object} response.Response{data=[]res.Relationship}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/mute/batch-status [post]
func (a *SocialApi) MuteBatch(c *gin.Context) { batch(c) }

// SubscriptionBatch
// @Tags SocialApp
// @Summary 批量查询本人关系（最多100个目标，自动去重）
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.TargetBatch true "请求参数"
// @Success 200 {object} response.Response{data=[]res.Relationship}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/live/subscription/batch-status [post]
func (a *SocialApi) SubscriptionBatch(c *gin.Context) { batch(c) }

// FollowingList
// @Tags SocialApp
// @Summary 查询本人关注游标列表
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param cursor query string false "上页nextCursor"
// @Param limit query int false "1至100，默认20"
// @Param resourceType query string false "资源类型过滤（仅收藏、历史列表使用）"
// @Success 200 {object} response.Response{data=res.UserList}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/following/list [get]
func (a *SocialApi) FollowingList(c *gin.Context) { userList(c, "following") }

// FollowersList
// @Tags SocialApp
// @Summary 查询本人粉丝游标列表
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param cursor query string false "上页nextCursor"
// @Param limit query int false "1至100，默认20"
// @Param resourceType query string false "资源类型过滤（仅收藏、历史列表使用）"
// @Success 200 {object} response.Response{data=res.UserList}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/followers/list [get]
func (a *SocialApi) FollowersList(c *gin.Context) { userList(c, "followers") }

// MutualList
// @Tags SocialApp
// @Summary 查询本人互相关注游标列表
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param cursor query string false "上页nextCursor"
// @Param limit query int false "1至100，默认20"
// @Param resourceType query string false "资源类型过滤（仅收藏、历史列表使用）"
// @Success 200 {object} response.Response{data=res.UserList}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/follow/mutual [get]
func (a *SocialApi) MutualList(c *gin.Context) { userList(c, "mutual") }

// BlockList
// @Tags SocialApp
// @Summary 查询本人黑名单游标列表
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param cursor query string false "上页nextCursor"
// @Param limit query int false "1至100，默认20"
// @Param resourceType query string false "资源类型过滤（仅收藏、历史列表使用）"
// @Success 200 {object} response.Response{data=res.UserList}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/block/list [get]
func (a *SocialApi) BlockList(c *gin.Context) { userList(c, "block") }

// MuteList
// @Tags SocialApp
// @Summary 查询本人屏蔽游标列表
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param cursor query string false "上页nextCursor"
// @Param limit query int false "1至100，默认20"
// @Param resourceType query string false "资源类型过滤（仅收藏、历史列表使用）"
// @Success 200 {object} response.Response{data=res.UserList}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/mute/list [get]
func (a *SocialApi) MuteList(c *gin.Context) { userList(c, "mute") }

// SubscriptionList
// @Tags SocialApp
// @Summary 查询本人提醒订阅游标列表
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param cursor query string false "上页nextCursor"
// @Param limit query int false "1至100，默认20"
// @Param resourceType query string false "资源类型过滤（仅收藏、历史列表使用）"
// @Success 200 {object} response.Response{data=res.UserList}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/live/subscription/list [get]
func (a *SocialApi) SubscriptionList(c *gin.Context) { userList(c, "subscription") }

// Status
// @Tags SocialApp
// @Summary 查询本人和目标的关注、拉黑、屏蔽及提醒状态
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param accountNo query string false "用户公开编号（与anchorNo二选一）"
// @Param anchorNo query string false "主播公开编号"
// @Success 200 {object} response.Response{data=[]res.Relationship}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/follow/status [get]
func (a *SocialApi) Status(c *gin.Context) {
	var r req.Target
	if !bind(c, &r) {
		return
	}
	data, err := socialService.BatchStatus(c.Request.Context(), actor(c), []req.Target{r})
	respond(c, data, err)
}

// Counters
// @Tags SocialApp
// @Summary 查询现存关系总计数（包含列表中被隐藏的不可用账号）
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param accountNo query string false "用户公开编号（与anchorNo二选一）"
// @Param anchorNo query string false "主播公开编号"
// @Success 200 {object} response.Response{data=res.Counters}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/follow/counters [get]
func (a *SocialApi) Counters(c *gin.Context) {
	var r req.Target
	if !bind(c, &r) {
		return
	}
	data, err := socialService.Counters(c.Request.Context(), r)
	respond(c, data, err)
}

// Common
// @Tags SocialApp
// @Summary 查询本人和指定用户共同关注
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param accountNo query string false "用户公开编号（与anchorNo二选一）"
// @Param anchorNo query string false "主播公开编号"
// @Param cursor query string false "上页nextCursor"
// @Param limit query int false "1至100，默认20"
// @Param resourceType query string false "资源类型过滤（仅收藏、历史列表使用）"
// @Success 200 {object} response.Response{data=res.UserList}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/follow/common [get]
func (a *SocialApi) Common(c *gin.Context) {
	var r req.CommonSearch
	if !bind(c, &r) {
		return
	}
	id, err := socialService.Resolve(c.Request.Context(), r.Target)
	if err != nil {
		respond(c, nil, err)
		return
	}
	data, err := socialService.ListUsers(c.Request.Context(), actor(c), "common", r.CursorSearch, id)
	respond(c, data, err)
}

// Mute
// @Tags SocialApp
// @Summary 设置或取消指定范围屏蔽；不改变关注关系
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.Mute true "请求参数"
// @Success 200 {object} response.Response{data=res.Change}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/mute [post]
func (a *SocialApi) Mute(c *gin.Context) {
	var r req.Mute
	if !bind(c, &r) {
		return
	}
	data, err := socialService.ChangeRelationship(c.Request.Context(), actor(c), r.Target, svc.Mute, r.Scope)
	respond(c, data, err)
}

// Unmute
// @Tags SocialApp
// @Summary 设置或取消指定范围屏蔽；不改变关注关系
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.Mute true "请求参数"
// @Success 200 {object} response.Response{data=res.Change}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/unmute [post]
func (a *SocialApi) Unmute(c *gin.Context) {
	var r req.Mute
	if !bind(c, &r) {
		return
	}
	data, err := socialService.ChangeRelationship(c.Request.Context(), actor(c), r.Target, svc.Unmute, r.Scope)
	respond(c, data, err)
}

// Favorite
// @Tags SocialApp
// @Summary 收藏或取消收藏公开编号指定的可访问资源
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.Resource true "请求参数"
// @Success 200 {object} response.Response{data=res.Change}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/favorite [post]
func (a *SocialApi) Favorite(c *gin.Context) {
	var r req.Resource
	if !bind(c, &r) {
		return
	}
	data, err := socialService.Favorite(c.Request.Context(), actor(c), r, true)
	respond(c, data, err)
}

// Unfavorite
// @Tags SocialApp
// @Summary 收藏或取消收藏公开编号指定的可访问资源
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.Resource true "请求参数"
// @Success 200 {object} response.Response{data=res.Change}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/unfavorite [post]
func (a *SocialApi) Unfavorite(c *gin.Context) {
	var r req.Resource
	if !bind(c, &r) {
		return
	}
	data, err := socialService.Favorite(c.Request.Context(), actor(c), r, false)
	respond(c, data, err)
}

// FavoriteBatch
// @Tags SocialApp
// @Summary 批量查询本人收藏状态；不可见资源返回false
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.ResourceBatch true "请求参数"
// @Success 200 {object} response.Response{data=[]res.FavoriteStatus}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/favorite/batch-status [post]
func (a *SocialApi) FavoriteBatch(c *gin.Context) {
	var r req.ResourceBatch
	if !bind(c, &r) {
		return
	}
	data, err := socialService.FavoriteStatus(c.Request.Context(), actor(c), r.Resources)
	respond(c, data, err)
}

// HistoryOpen
// @Tags SocialApp
// @Summary 开启观看会话，服务端版本确定多设备写入权
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.HistoryOpen true "请求参数"
// @Success 200 {object} response.Response{data=res.WatchSession}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/history/open [post]
func (a *SocialApi) HistoryOpen(c *gin.Context) {
	var r req.HistoryOpen
	if !bind(c, &r) {
		return
	}
	data, err := socialService.OpenHistory(c.Request.Context(), actor(c), r)
	respond(c, data, err)
}

// HistoryReport
// @Tags SocialApp
// @Summary 上报心跳或严格持久化checkpoint；seq递增，毫秒进度允许回退
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.HistoryReport true "请求参数"
// @Success 200 {object} response.Response{data=res.WatchAccepted}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/history/report [post]
func (a *SocialApi) HistoryReport(c *gin.Context) {
	var r req.HistoryReport
	if !bind(c, &r) {
		return
	}
	data, err := socialService.ReportHistory(c.Request.Context(), actor(c), r)
	respond(c, data, err)
}

// HistoryList
// @Tags SocialApp
// @Description 列表项复用用户公开资料，isLive 仅表示当前观众可访问的有效直播。resourceId 保留用于删除观看记录。
// @Summary 查询本人最近观看的用户公开资料及当前直播状态
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param cursor query string false "上页nextCursor"
// @Param limit query int false "1至100，默认20"
// @Param resourceType query string false "默认live_room，目前仅支持live_room"
// @Success 200 {object} response.Response{data=res.HistoryUserList}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/history/list [get]
func (a *SocialApi) HistoryList(c *gin.Context) {
	var r req.CursorSearch
	if !bind(c, &r) {
		return
	}
	data, err := socialService.HistoryList(c.Request.Context(), actor(c), r)
	respond(c, data, err)
}

// HistoryProgress
// @Tags SocialApp
// @Summary 批量获取本人已持久化观看进度
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.ResourceBatch true "请求参数"
// @Success 200 {object} response.Response{data=res.ResourceList}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/history/progress/batch [post]
func (a *SocialApi) HistoryProgress(c *gin.Context) {
	var r req.ResourceBatch
	if !bind(c, &r) {
		return
	}
	data, err := socialService.HistoryProgress(c.Request.Context(), actor(c), r.Resources)
	respond(c, data, err)
}

// HistoryDelete
// @Tags SocialApp
// @Summary 删除本人的指定观看记录，旧缓冲不能重建
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param data body req.Resource true "请求参数"
// @Success 200 {object} response.Response{data=object}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/history/delete [post]
func (a *SocialApi) HistoryDelete(c *gin.Context) {
	var r req.Resource
	if !bind(c, &r) {
		return
	}
	err := socialService.DeleteHistory(c.Request.Context(), actor(c), &r)
	respond(c, struct{}{}, err)
}

// HistoryClear
// @Tags SocialApp
// @Summary 清空本人观看历史并撤销旧缓冲代际
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Success 200 {object} response.Response{data=object}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/history/clear [post]
func (a *SocialApi) HistoryClear(c *gin.Context) {
	err := socialService.DeleteHistory(c.Request.Context(), actor(c), nil)
	respond(c, struct{}{}, err)
}

// FollowingOnline
// @Tags SocialApp
// @Summary 游标扫描本人关注中正在直播且有权访问的主播
// @Description 身份仅来自APP JWT。成功code=0；错误46001参数、46002账号、46003自己、46004拉黑、46005未关注、46006不可访问资源、46007未接入类型、46008限流、46009依赖故障、46010会话冲突。所有时间与进度为毫秒。目标示例：{"accountNo":"10000001"}；accountNo/anchorNo二选一。列表最多100条，过滤后空页仍应跟随nextCursor。
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param cursor query string false "上页nextCursor"
// @Param limit query int false "1至100，默认20"
// @Param resourceType query string false "资源类型过滤（仅收藏、历史列表使用）"
// @Success 200 {object} response.Response{data=res.OnlineList}
// @Failure 401 {object} response.Response "未登录或JWT已撤销"
// @Router /v1/app/social/live/following-online [get]
func (a *SocialApi) FollowingOnline(c *gin.Context) {
	var r req.CursorSearch
	if !bind(c, &r) {
		return
	}
	data, err := socialService.FollowingOnline(c.Request.Context(), actor(c), r)
	respond(c, data, err)
}
