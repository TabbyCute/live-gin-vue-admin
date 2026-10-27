package social

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"tb_live_module/config"
	model "tb_live_module/model/social"
	req "tb_live_module/model/social/request"
	res "tb_live_module/model/social/response"
	repo "tb_live_module/repository/social"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type BusinessError struct {
	Code    int
	Message string
}

func (e *BusinessError) Error() string { return e.Message }

var (
	ErrInvalid         = &BusinessError{46001, "社交参数无效"}
	ErrUnavailableUser = &BusinessError{46002, "账号不存在或不可用"}
	ErrSelf            = &BusinessError{46003, "不能对自己执行此操作"}
	ErrBlocked         = &BusinessError{46004, "存在拉黑关系"}
	ErrFollowRequired  = &BusinessError{46005, "请先关注主播"}
	ErrResource        = &BusinessError{46006, "资源不存在或无权访问"}
	ErrUnsupported     = &BusinessError{46007, "此资源类型尚未接入"}
	ErrRateLimited     = &BusinessError{46008, "请求过于频繁，请稍后重试"}
	ErrUnavailable     = &BusinessError{46009, "社交服务暂不可用"}
	ErrConflict        = &BusinessError{46010, "状态冲突，请重新获取播放会话"}
)

// Directory 由账号/主播所属模块实现。正常用户的列表展示仅返回公开字段。
type Directory interface {
	Resolve(context.Context, *gorm.DB, []req.Target) ([]uint64, error)
	Validate(context.Context, *gorm.DB, []uint64) error
	Profiles(context.Context, *gorm.DB, uint64, []uint64) (map[uint64]res.User, error)
	IsAnchor(context.Context, *gorm.DB, uint64) (bool, error)
	Online(context.Context, *gorm.DB, uint64, []uint64) ([]res.OnlineRoom, error)
	LiveEventValid(context.Context, *gorm.DB, uint64, string) (bool, error)
}

// ResourceProvider 必须批量校验可见性，不得在事务中访问外部服务。
type ResourceProvider interface {
	Resolve(context.Context, *gorm.DB, uint64, []req.Resource) (map[req.Resource]res.Resource, error)
}
type Notification struct {
	EventID        string
	UserID         uint64
	AnchorUserID   uint64
	SessionNo      string
	IdempotencyKey string
}
type NotificationSink interface {
	Name() string
	Deliver(context.Context, Notification) error
}
type Service struct {
	DB        *gorm.DB
	Redis     redis.UniversalClient
	Directory Directory
	Resources map[model.ResourceType]ResourceProvider
	Config    config.Social
	Log       *zap.Logger
	Sink      NotificationSink
	Metrics   Metrics
}

func (s *Service) db(ctx context.Context) *gorm.DB {
	return s.DB.WithContext(ctx).Session(&gorm.Session{NowFunc: func() time.Time { return time.Now().UTC() }})
}
func (s *Service) ready() error {
	if s.DB == nil || s.Directory == nil {
		return ErrUnavailable
	}
	return nil
}
func (s *Service) repository() repo.Repository {
	return repo.Repository{DB: s.DB, OnRetry: func(n uint16) {
		s.Metrics.Retries.Add(1)
		if n == 1213 {
			s.Metrics.Deadlocks.Add(1)
		}
	}}
}

var publicNumber = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func validResource(r req.Resource) bool {
	return r.ResourceType.Valid() && publicNumber.MatchString(r.ResourceID)
}
func (s *Service) Resolve(ctx context.Context, t req.Target) (uint64, error) {
	ids, err := s.resolveMany(ctx, []req.Target{t})
	if err != nil {
		return 0, err
	}
	return ids[0], nil
}
func (s *Service) resolveMany(ctx context.Context, ts []req.Target) ([]uint64, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if len(ts) == 0 || len(ts) > 100 {
		return nil, ErrInvalid
	}
	for _, t := range ts {
		if (t.AccountNo == "") == (t.AnchorNo == "") || len(t.AccountNo) > 32 || len(t.AnchorNo) > 32 || !publicNumber.MatchString(t.AccountNo+t.AnchorNo) {
			return nil, ErrInvalid
		}
	}
	return s.Directory.Resolve(ctx, s.db(ctx), ts)
}
func (s *Service) resources(ctx context.Context, db *gorm.DB, actor uint64, refs []req.Resource) (map[req.Resource]res.Resource, error) {
	if len(refs) == 0 || len(refs) > 100 {
		return nil, ErrInvalid
	}
	groups := map[model.ResourceType][]req.Resource{}
	seen := map[req.Resource]bool{}
	for _, r := range refs {
		if !validResource(r) {
			return nil, ErrInvalid
		}
		if !seen[r] {
			groups[r.ResourceType] = append(groups[r.ResourceType], r)
			seen[r] = true
		}
	}
	result := map[req.Resource]res.Resource{}
	for typ, group := range groups {
		provider := s.Resources[typ]
		if provider == nil {
			return nil, ErrUnsupported
		}
		rows, err := provider.Resolve(ctx, db, actor, group)
		if err != nil {
			return nil, err
		}
		for k, v := range rows {
			result[k] = v
		}
	}
	return result, nil
}
func event(tx *gorm.DB, typ string, a, b uint64, object, action, key string) error {
	id := uuid.NewString()
	if key == "" {
		key = id
	}
	return tx.Create(&model.SocialOutboxEvent{EventID: id, BusinessKey: key, EventType: typ, Version: 1, UserID: a, TargetUserID: b, ObjectNo: object, Action: action, Status: model.EventPending}).Error
}

// cursor 用时间毫秒+ID消除同一毫秒歧义；ID 是不透明游标内部信息，不是客户端对象参数。
type cursor struct {
	At int64
	ID uint64
}

func encodeCursor(at int64, id uint64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d:%d", at, id)))
}
func decodeCursor(raw string) (cursor, error) {
	if raw == "" {
		return cursor{}, nil
	}
	if len(raw) > 128 {
		return cursor{}, ErrInvalid
	}
	b, e := base64.RawURLEncoding.Strict().DecodeString(raw)
	if e != nil {
		return cursor{}, ErrInvalid
	}
	p := strings.Split(string(b), ":")
	if len(p) != 2 {
		return cursor{}, ErrInvalid
	}
	at, e := strconv.ParseInt(p[0], 10, 64)
	if e != nil || at < 0 {
		return cursor{}, ErrInvalid
	}
	id, e := strconv.ParseUint(p[1], 10, 64)
	if e != nil || id == 0 || id > repo.MaxSafeInteger {
		return cursor{}, ErrInvalid
	}
	return cursor{at, id}, nil
}
func pageLimit(n int) (int, error) {
	if n == 0 {
		return 20, nil
	}
	if n < 1 || n > 100 {
		return 0, ErrInvalid
	}
	return n, nil
}
func uniqueIDs(ids []uint64) []uint64 {
	out := make([]uint64, 0, len(ids))
	m := map[uint64]bool{}
	for _, id := range ids {
		if id > 0 && !m[id] {
			m[id] = true
			out = append(out, id)
		}
	}
	return out
}
func isBusiness(err error) bool { var b *BusinessError; return errors.As(err, &b) }
