package live

import (
	"errors"

	"tb_live_module/global"
	"tb_live_module/middleware"
	"tb_live_module/model/common/response"
	liveModel "tb_live_module/model/live"
	liveReq "tb_live_module/model/live/request"
	liveRes "tb_live_module/model/live/response"
	liveService "tb_live_module/service/live"
	"tb_live_module/utils"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type UserApi struct{}

// Register
// @Tags        LiveAccount
// @Summary     客户端账号注册
// @Description 创建独立于管理后台账号体系的客户端账号；用户名不区分大小写
// @Accept      application/json
// @Produce     application/json
// @Param       data  body      liveReq.Register                                      true  "用户名、密码、昵称和头像"
// @Success     200   {object}  response.Response{data=liveRes.RegisterResponse}      "注册成功"
// @Failure     200   {object}  response.Response                                     "参数错误或用户名已注册"
// @Router      /v1/app/live/user/register [post]
func (u *UserApi) Register(c *gin.Context) {
	var input liveReq.Register
	if err := c.ShouldBindJSON(&input); err != nil {
		response.FailWithMessage("注册参数错误: "+err.Error(), c)
		return
	}
	account, err := accountService.Register(input)
	if err != nil {
		if errors.Is(err, liveService.ErrAccountExists) || errors.Is(err, liveService.ErrInvalidUsername) || errors.Is(err, liveService.ErrInvalidPassword) {
			response.FailWithMessage(err.Error(), c)
			return
		}
		global.GVA_LOG.Error("客户端账号注册失败", zap.Error(err))
		response.FailWithMessage("注册失败", c)
		return
	}
	response.OkWithDetailed(liveRes.RegisterResponse{Account: accountDTO(account)}, "注册成功", c)
}

// Login
// @Tags        LiveAccount
// @Summary     客户端账号登录
// @Description 校验客户端账号密码并签发客户端专用 JWT
// @Accept      application/json
// @Produce     application/json
// @Param       data  body      liveReq.Login                                      true  "用户名和密码"
// @Success     200   {object}  response.Response{data=liveRes.LoginResponse}      "登录成功"
// @Failure     200   {object}  response.Response                                  "参数错误、凭证错误或账号禁用"
// @Router      /v1/app/live/user/login [post]
func (u *UserApi) Login(c *gin.Context) {
	var input liveReq.Login
	if err := c.ShouldBindJSON(&input); err != nil {
		response.FailWithMessage("登录参数错误: "+err.Error(), c)
		return
	}
	account, err := accountService.Login(input)
	if err != nil {
		switch {
		case errors.Is(err, liveService.ErrInvalidCredentials):
			response.FailWithMessage(liveService.ErrInvalidCredentials.Error(), c)
		case errors.Is(err, liveService.ErrAccountDisabled):
			response.FailWithMessage(liveService.ErrAccountDisabled.Error(), c)
		default:
			global.GVA_LOG.Error("客户端账号登录失败", zap.Error(err))
			response.FailWithMessage("登录失败", c)
		}
		return
	}

	token, claims, err := utils.NewAppJWT().CreateToken(account.ID, account.Username)
	if err != nil {
		global.GVA_LOG.Error("签发客户端 token 失败", zap.Error(err))
		response.FailWithMessage("登录失败", c)
		return
	}
	response.OkWithDetailed(liveRes.LoginResponse{
		Account:   accountDTO(*account),
		Token:     token,
		ExpiresAt: claims.ExpiresAt.Unix() * 1000,
	}, "登录成功", c)
}

// Info
// @Tags        LiveAccount
// @Summary     获取当前客户端账号
// @Description 从客户端 JWT 上下文获取账号 ID，并返回安全账号信息
// @Security    AppBearerAuth
// @Produce     application/json
// @Success     200  {object}  response.Response{data=liveRes.AccountResponse}  "获取成功"
// @Failure     401  {object}  response.Response                                "客户端 token 缺失、无效或账号禁用"
// @Router      /v1/app/live/user/info [get]
func (u *UserApi) Info(c *gin.Context) {
	accountID, ok := middleware.GetAppAccountID(c)
	if !ok {
		response.NoAuth("未获取到客户端登录信息", c)
		return
	}
	account, err := accountService.GetByID(accountID)
	if err != nil {
		global.GVA_LOG.Error("获取客户端账号信息失败", zap.Error(err))
		response.FailWithMessage("获取账号信息失败", c)
		return
	}
	response.OkWithDetailed(liveRes.AccountResponse{Account: accountDTO(*account)}, "获取成功", c)
}

func accountDTO(account liveModel.LiveAccount) liveRes.Account {
	return liveRes.Account{
		ID:        account.ID,
		Username:  account.Username,
		Nickname:  account.Nickname,
		Avatar:    account.Avatar,
		Status:    account.Status,
		CreatedAt: account.CreatedAt,
	}
}
