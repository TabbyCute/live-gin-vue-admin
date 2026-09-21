package utils

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"tb_live_module/global"
	liveReq "tb_live_module/model/live/request"

	jwt "github.com/golang-jwt/jwt/v5"
)

const appJWTAudience = "live-app"

var (
	ErrAppTokenExpired          = errors.New("客户端 token 已过期")
	ErrAppTokenInvalid          = errors.New("客户端 token 无效")
	ErrAppJWTConfigInvalid      = errors.New("客户端 JWT 配置无效")
	ErrAppJWTSigningKeyNotSplit = errors.New("客户端 JWT 密钥不能与管理后台相同")
)

type AppJWT struct {
	SigningKey []byte
}

func NewAppJWT() *AppJWT {
	return &AppJWT{SigningKey: []byte(global.GVA_CONFIG.JWTApp.SigningKey)}
}

func (j *AppJWT) CreateToken(accountID uint, username string) (string, liveReq.AppClaims, error) {
	if err := j.validateConfig(); err != nil {
		return "", liveReq.AppClaims{}, err
	}
	expiresIn, err := ParseDuration(global.GVA_CONFIG.JWTApp.ExpiresTime)
	if err != nil || expiresIn <= 0 {
		return "", liveReq.AppClaims{}, fmt.Errorf("%w: expires-time", ErrAppJWTConfigInvalid)
	}

	now := time.Now()
	claims := liveReq.AppClaims{
		AccountID: accountID,
		Username:  username,
		TokenType: liveReq.AppTokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Audience:  jwt.ClaimStrings{appJWTAudience},
			ExpiresAt: jwt.NewNumericDate(now.Add(expiresIn)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-time.Second)),
			Issuer:    global.GVA_CONFIG.JWTApp.Issuer,
			Subject:   strconv.FormatUint(uint64(accountID), 10),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString(j.SigningKey)
	return signedToken, claims, err
}

func (j *AppJWT) ParseToken(tokenString string) (*liveReq.AppClaims, error) {
	if err := j.validateConfig(); err != nil {
		return nil, err
	}
	claims := new(liveReq.AppClaims)
	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(*jwt.Token) (interface{}, error) { return j.SigningKey, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(global.GVA_CONFIG.JWTApp.Issuer),
		jwt.WithAudience(appJWTAudience),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrAppTokenExpired
		}
		return nil, ErrAppTokenInvalid
	}
	if token == nil || !token.Valid || claims.AccountID == 0 || claims.TokenType != liveReq.AppTokenType || claims.Subject != strconv.FormatUint(uint64(claims.AccountID), 10) {
		return nil, ErrAppTokenInvalid
	}
	return claims, nil
}

func (j *AppJWT) validateConfig() error {
	appConfig := global.GVA_CONFIG.JWTApp
	if len(j.SigningKey) < 16 || appConfig.Issuer == "" {
		return ErrAppJWTConfigInvalid
	}
	if appConfig.SigningKey == global.GVA_CONFIG.JWT.SigningKey {
		return ErrAppJWTSigningKeyNotSplit
	}
	return nil
}
