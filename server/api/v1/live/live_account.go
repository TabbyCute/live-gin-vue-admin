package live

import (
	"errors"
	"time"

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
// @Description 使用E.164手机号、6位超级验证码和密码注册；系统自动生成数字账号编号和内部用户名。当前为短信接入前的联调实现，验证码只允许注册。
// @Accept      application/json
// @Produce     application/json
// @Param       data  body      liveReq.Register                                      true  "手机号、验证码、密码及可选昵称头像"
// @Success     200   {object}  response.Response{data=liveRes.RegisterResponse}      "注册成功"
// @Failure     200   {object}  response.Response                                     "参数错误、验证码错误或手机号已注册"
// @Failure     429   {object}  response.Response                                     "请求过于频繁"
// @Router      /v1/app/live/user/register [post]
func (u *UserApi) Register(c *gin.Context) {
	var input liveReq.Register
	if err := c.ShouldBindJSON(&input); err != nil {
		response.FailWithMessage("注册参数错误: "+err.Error(), c)
		return
	}
	account, err := accountService.RegisterWithContext(c.Request.Context(), input)
	if err != nil {
		if errors.Is(err, liveService.ErrPhoneExists) || errors.Is(err, liveService.ErrInvalidPhone) || errors.Is(err, liveService.ErrInvalidSMSCode) || errors.Is(err, liveService.ErrRegistrationUnavailable) || errors.Is(err, liveService.ErrInvalidPassword) || errors.Is(err, liveService.ErrInvalidProfile) {
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
// @Description 仅支持已验证手机号与密码登录，不支持用户名或账号编号登录；超级验证码不能代替密码。
// @Accept      application/json
// @Produce     application/json
// @Param       data  body      liveReq.Login                                      true  "E.164手机号和密码"
// @Success     200   {object}  response.Response{data=liveRes.LoginResponse}      "登录成功"
// @Failure     200   {object}  response.Response                                  "参数错误、凭证错误或账号禁用"
// @Router      /v1/app/live/user/login [post]
func (u *UserApi) Login(c *gin.Context) {
	var input liveReq.Login
	if err := c.ShouldBindJSON(&input); err != nil {
		response.FailWithMessage("登录参数错误: "+err.Error(), c)
		return
	}
	account, err := accountService.LoginWithContext(c.Request.Context(), input)
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

	token, claims, err := utils.NewAppJWT().CreateToken(account.ID, account.Username, account.TokenVersion)
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

// SendSMS
// @Tags LiveAccount
// @Summary 注册短信占位接口
// @Description 校验参数和限流后等待2秒返回ok，不发送短信、不生成或保存验证码；purpose可省略或为register。请求取消时立即停止等待。
// @Accept application/json
// @Produce application/json
// @Param data body liveReq.AccountSMSSendReq true "手机号及可选注册用途"
// @Success 200 {object} response.Response "ok"
// @Failure 429 {object} response.Response "请求过于频繁"
// @Router /v1/app/live/user/sms/send [post]
func (u *UserApi) SendSMS(c *gin.Context) {
	var input liveReq.AccountSMSSendReq
	if err := c.ShouldBindJSON(&input); err != nil {
		response.FailWithMessage("短信参数错误", c)
		return
	}
	if _, err := utils.NormalizeAccountPhone(input.Phone); err != nil {
		response.FailWithMessage(liveService.ErrInvalidPhone.Error(), c)
		return
	}
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-c.Request.Context().Done():
		return
	case <-timer.C:
		if c.Request.Context().Err() == nil {
			response.OkWithMessage("ok", c)
		}
	}
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

// UpdateProfile
// @Tags LiveAccount
// @Summary 修改当前客户端账号资料
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param data body liveReq.AccountProfileUpdateReq true "公开资料白名单"
// @Success 200 {object} response.Response "修改成功"
// @Router /v1/app/live/user/profile [patch]
func (u *UserApi) UpdateProfile(c *gin.Context) {
	accountID, ok := middleware.GetAppAccountID(c)
	if !ok {
		response.NoAuth("未获取到客户端登录信息", c)
		return
	}
	var req liveReq.AccountProfileUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage("资料参数错误: "+err.Error(), c)
		return
	}
	if err := accountService.UpdateProfile(accountID, req); err != nil {
		if errors.Is(err, liveService.ErrInvalidProfile) {
			response.FailWithMessage(err.Error(), c)
			return
		}
		global.GVA_LOG.Error("修改客户端资料失败", zap.Error(err))
		response.FailWithMessage("修改资料失败", c)
		return
	}
	response.OkWithMessage("修改成功", c)
}

// ChangePassword
// @Tags LiveAccount
// @Summary 修改当前客户端账号密码并轮换JWT版本
// @Security AppBearerAuth
// @Accept application/json
// @Produce application/json
// @Param data body liveReq.AccountPasswordChangeReq true "旧密码和新密码"
// @Success 200 {object} response.Response{data=liveRes.PasswordChangeResponse} "修改成功并签发新token"
// @Router /v1/app/live/user/password/change [post]
func (u *UserApi) ChangePassword(c *gin.Context) {
	accountID, ok := middleware.GetAppAccountID(c)
	if !ok {
		response.NoAuth("未获取到客户端登录信息", c)
		return
	}
	var req liveReq.AccountPasswordChangeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage("密码参数错误: "+err.Error(), c)
		return
	}
	account, err := accountService.ChangePassword(accountID, req)
	if err != nil {
		if errors.Is(err, liveService.ErrInvalidCredentials) || errors.Is(err, liveService.ErrInvalidPassword) {
			response.FailWithMessage(err.Error(), c)
			return
		}
		global.GVA_LOG.Error("修改客户端密码失败", zap.Error(err))
		response.FailWithMessage("修改密码失败", c)
		return
	}
	token, claims, err := utils.NewAppJWT().CreateToken(account.ID, account.Username, account.TokenVersion)
	if err != nil {
		global.GVA_LOG.Error("修改密码后签发客户端token失败", zap.Error(err))
		response.FailWithMessage("密码已修改，请重新登录", c)
		return
	}
	response.OkWithDetailed(liveRes.PasswordChangeResponse{Token: token, ExpiresAt: claims.ExpiresAt.UnixMilli()}, "修改成功", c)
}

// PublicProfile
// @Tags LiveAccount
// @Summary 获取用户公开资料
// @Description 复用UserPublicProfile；isLive表示当前用户可访问的有效直播，为false时省略roomNo和sessionNo。不含内部ID、手机号、实名等私密字段。
// @Security AppBearerAuth
// @Produce application/json
// @Param accountNo query string true "对外账号编号"
// @Success 200 {object} response.Response{data=liveRes.AccountPublicProfileResp} "获取成功"
// @Router /v1/app/live/user/public-profile [get]
func (u *UserApi) PublicProfile(c *gin.Context) {
	var req liveReq.AccountPublicProfileReq
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage("查询参数错误: "+err.Error(), c)
		return
	}
	viewer, _ := middleware.GetAppAccountID(c)
	profile, err := accountService.GetViewerPublicProfile(c.Request.Context(), uint64(viewer), req.AccountNo)
	if err != nil {
		if errors.Is(err, liveService.ErrAccountNotFound) {
			response.FailWithMessage(err.Error(), c)
			return
		}
		global.GVA_LOG.Error("获取用户公开资料失败", zap.Error(err))
		response.FailWithMessage("获取用户资料失败", c)
		return
	}
	response.OkWithDetailed(profile, "获取成功", c)
}

func accountDTO(account liveModel.LiveAccount) liveRes.Account {
	return liveRes.Account{
		ID: account.ID, AccountNo: account.AccountNo, Username: account.Username,
		PhoneMasked: liveService.MaskAccountPhone(account.Phone), PhoneVerified: account.Phone != nil && account.PhoneVerifiedAt > 0,
		Nickname: account.Nickname, Avatar: account.Avatar, Signature: account.Signature, Gender: account.Gender,
		Birthday: liveService.FormatAccountBirthday(account.Birthday), CountryCode: account.CountryCode,
		RegionCode: account.RegionCode, CityCode: account.CityCode, Language: account.Language,
		RealNameStatus: account.RealNameStatus, RealNameVerifiedAt: account.RealNameVerifiedAt,
		Status: account.Status, BanUntil: account.BanUntil, LastLoginAt: account.LastLoginAt, CreatedAt: account.CreatedAt,
	}
}
