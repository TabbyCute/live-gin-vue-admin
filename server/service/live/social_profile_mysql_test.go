package live

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"tb_live_module/global"
	liveModel "tb_live_module/model/live"
	socialModel "tb_live_module/model/social"
	socialReq "tb_live_module/model/social/request"
	socialService "tb_live_module/service/social"
)

// 独立测试库防止并行package测试相互清表；只允许固定的本机临时MySQL。
func TestMySQLPublicProfilePlansAndBatchLoad(t *testing.T) {
	dsn := os.Getenv("SOCIAL_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("requires isolated MySQL")
	}
	cfg, err := driver.ParseDSN(dsn)
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:13316", cfg.Addr)
	require.True(t, strings.HasPrefix(cfg.DBName, "social_test_"))
	cfg.DBName = ""
	admin, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := admin.DB()
	require.NoError(t, err)
	defer pool.Close()
	const database = "social_test_public_profiles"
	require.NoError(t, admin.Exec("CREATE DATABASE IF NOT EXISTS "+database).Error)
	defer admin.Exec("DROP DATABASE " + database)
	cfg.DBName = database
	db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	connections, err := db.DB()
	require.NoError(t, err)
	defer connections.Close()
	connections.SetMaxOpenConns(16)
	connections.SetMaxIdleConns(16)
	require.NoError(t, db.AutoMigrate(append(socialModel.Models(), &liveModel.LiveAccount{}, &liveModel.LiveAnchor{}, &liveModel.LiveRoom{}, &liveModel.LiveSession{})...))
	accounts := make([]liveModel.LiveAccount, 1001)
	for i := range accounts {
		id := uint(1000 + i)
		accounts[i] = liveModel.LiveAccount{GVA_MODEL: global.GVA_MODEL{ID: id}, AccountNo: fmt.Sprint(id), Username: fmt.Sprint(id), Nickname: "公开用户", Status: liveModel.LiveAccountStatusNormal}
	}
	require.NoError(t, db.CreateInBatches(accounts, 100).Error)
	ids := make([]uint64, 100)
	for i := 0; i < 1000; i++ {
		id := uint(1000 + i)
		if i < len(ids) {
			ids[i] = uint64(id)
		}
		a := liveModel.LiveAnchor{GVA_MODEL: global.GVA_MODEL{ID: id}, UserId: uint64(id), AnchorNo: fmt.Sprint(id), Status: liveModel.AnchorStatusNormal, ApplyStatus: liveModel.AnchorApplyStatusApproved, LivePermission: liveModel.AnchorPermissionEnabled}
		require.NoError(t, db.Create(&a).Error)
		r := liveModel.LiveRoom{GVA_MODEL: global.GVA_MODEL{ID: id}, AnchorId: id, RoomNo: fmt.Sprint(id), StreamName: fmt.Sprint(id), Status: liveModel.LiveRoomStatusNormal, Visibility: liveModel.LiveRoomVisibilityFollowers, LiveStatus: liveModel.LiveRoomLiving, CurrentSessionId: id}
		require.NoError(t, db.Create(&r).Error)
		session := liveModel.LiveSession{GVA_MODEL: global.GVA_MODEL{ID: id}, RoomId: id, AnchorId: id, SessionNo: fmt.Sprint(id), Status: liveModel.LiveSessionLiving}
		require.NoError(t, db.Create(&session).Error)
		if i < len(ids) {
			require.NoError(t, db.Create(&socialModel.SocialFollow{UserID: 2000, TargetUserID: uint64(id)}).Error)
		}
	}
	require.NoError(t, db.Exec("ANALYZE TABLE live_anchor,live_room,live_session,live_account,social_follow,social_block").Error)
	ctx := context.Background()
	s := &socialService.Service{DB: db, Directory: SocialDirectory{}, Resources: map[socialModel.ResourceType]socialService.ResourceProvider{socialModel.ResourceLiveRoom: SocialRoomResource{}}}
	list, err := s.ListUsers(ctx, 2000, "following", socialReq.CursorSearch{Limit: 100}, 0)
	require.NoError(t, err)
	require.Len(t, list.List, 100)
	for _, u := range list.List {
		require.True(t, u.IsLive)
		require.NotEmpty(t, u.RoomNo)
	}
	for _, r := range []socialReq.Resource{{ResourceType: socialModel.ResourceLiveRoom, ResourceID: "1000"}, {ResourceType: socialModel.ResourceLiveRoom, ResourceID: "1001"}} {
		_, err := s.OpenHistory(ctx, 2000, socialReq.HistoryOpen{Resource: r, RequestID: "mysql-history-1"})
		require.NoError(t, err)
	}
	history, err := s.HistoryList(ctx, 2000, socialReq.CursorSearch{})
	require.NoError(t, err)
	require.Len(t, history.List, 2)
	require.True(t, history.List[0].IsLive)
	statement := liveAccessQuery(ctx, db.Session(&gorm.Session{DryRun: true}), 2000, true).Where("a.user_id IN ?", ids).Find(&[]LiveAccessSnapshot{}).Statement
	var plan []map[string]interface{}
	require.NoError(t, db.Raw("EXPLAIN "+statement.SQL.String(), statement.Vars...).Scan(&plan).Error)
	raw, err := json.Marshal(plan)
	require.NoError(t, err)
	t.Log("live profile EXPLAIN: " + string(raw))
	for _, row := range plan {
		// 账号、主播、房间、场次及权限均有索引，100个目标无需全表扫描。
		require.NotEqual(t, "ALL", fmt.Sprint(row["type"]), fmt.Sprint(row))
		if row["table"] == "s" {
			require.Equal(t, "PRIMARY", row["key"], "不能经主播索引扫描历史场次")
		}
	}
	if os.Getenv("SOCIAL_RUN_LOAD") != "1" {
		return
	}
	const requests = 400
	durations := make([]time.Duration, requests)
	errs := make([]error, requests)
	jobs := make(chan int)
	var wg sync.WaitGroup
	started := time.Now()
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				at := time.Now()
				_, errs[i] = s.ListUsers(ctx, 2000, "following", socialReq.CursorSearch{Limit: 100}, 0)
				durations[i] = time.Since(at)
			}
		}()
	}
	for i := 0; i < requests; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	elapsed := time.Since(started)
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	for _, err := range errs {
		require.NoError(t, err)
	}
	t.Logf("public user list 100: requests=%d concurrency=16 QPS=%.2f p95=%s p99=%s errors=0", requests, float64(requests)/elapsed.Seconds(), durations[379], durations[395])
}
