package live

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"tb_live_module/global"
	liveModel "tb_live_module/model/live"
	socialModel "tb_live_module/model/social"
	socialReq "tb_live_module/model/social/request"
	socialService "tb_live_module/service/social"
)

func TestRelationshipCacheRefreshesPublicProfileAndLiveState(t *testing.T) {
	addr := os.Getenv("SOCIAL_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("requires isolated Redis")
	}
	require.Equal(t, "127.0.0.1:16389", addr)
	s, _, session := setupAccessTest(t)
	ctx := context.Background()
	// DB14避免与social包并行测试使用的DB15互相清空。
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 14})
	t.Cleanup(func() { client.Close() })
	require.NoError(t, client.FlushDB(ctx).Err())
	s.Redis = client
	target := socialReq.Target{AccountNo: "host"}
	_, err := s.ChangeRelationship(ctx, 601, target, socialService.Follow, "")
	require.NoError(t, err)
	rows, err := s.BatchStatus(ctx, 601, []socialReq.Target{target})
	require.NoError(t, err)
	require.True(t, rows[0].IsLive)
	require.NoError(t, s.DB.Model(&session).Update("status", liveModel.LiveSessionEnded).Error)
	require.NoError(t, s.DB.Model(&liveModel.LiveAccount{}).Where("id = ?", 501).Update("nickname", "最新昵称").Error)
	rows, err = s.BatchStatus(ctx, 601, []socialReq.Target{target})
	require.NoError(t, err)
	require.Equal(t, uint64(1), s.Metrics.CacheHits.Load())
	require.True(t, rows[0].Following)
	require.Equal(t, "最新昵称", rows[0].Nickname)
	require.False(t, rows[0].IsLive)
	require.Empty(t, rows[0].RoomNo)
	require.Empty(t, rows[0].SessionNo)
}

func TestPublicProfilesShareUserFieldsAndLiveVisibility(t *testing.T) {
	cases := []struct {
		name   string
		viewer uint64
		change func(*testing.T, *socialService.Service, liveModel.LiveRoom, liveModel.LiveSession)
		live   bool
	}{
		{"followers_without_follow", 601, nil, false},
		{"anonymous_followers", 0, nil, false},
		{"owner", 501, nil, true},
		{"public", 601, func(t *testing.T, s *socialService.Service, r liveModel.LiveRoom, _ liveModel.LiveSession) {
			require.NoError(t, s.DB.Model(&r).Update("visibility", liveModel.LiveRoomVisibilityPublic).Error)
		}, true},
		{"following", 601, func(t *testing.T, s *socialService.Service, _ liveModel.LiveRoom, _ liveModel.LiveSession) {
			_, err := s.ChangeRelationship(context.Background(), 601, socialReq.Target{AccountNo: "host"}, socialService.Follow, "")
			require.NoError(t, err)
		}, true},
		{"blocked", 601, func(t *testing.T, s *socialService.Service, r liveModel.LiveRoom, _ liveModel.LiveSession) {
			require.NoError(t, s.DB.Model(&r).Update("visibility", liveModel.LiveRoomVisibilityPublic).Error)
			_, err := s.ChangeRelationship(context.Background(), 501, socialReq.Target{AccountNo: "viewer"}, socialService.Block, "")
			require.NoError(t, err)
		}, false},
		{"private", 601, func(t *testing.T, s *socialService.Service, r liveModel.LiveRoom, _ liveModel.LiveSession) {
			require.NoError(t, s.DB.Model(&r).Update("visibility", liveModel.LiveRoomVisibilityPrivate).Error)
		}, false},
		{"ended_session", 501, func(t *testing.T, s *socialService.Service, _ liveModel.LiveRoom, session liveModel.LiveSession) {
			require.NoError(t, s.DB.Model(&session).Update("status", liveModel.LiveSessionEnded).Error)
		}, false},
		{"mismatched_session", 501, func(t *testing.T, s *socialService.Service, _ liveModel.LiveRoom, session liveModel.LiveSession) {
			require.NoError(t, s.DB.Model(&session).Update("room_id", 99999).Error)
		}, false},
		{"preparing", 501, func(t *testing.T, s *socialService.Service, r liveModel.LiveRoom, _ liveModel.LiveSession) {
			require.NoError(t, s.DB.Model(&r).Update("live_status", liveModel.LiveRoomPreparing).Error)
		}, false},
		{"revoked_permission", 501, func(t *testing.T, s *socialService.Service, r liveModel.LiveRoom, _ liveModel.LiveSession) {
			require.NoError(t, s.DB.Model(&liveModel.LiveAnchor{}).Where("id = ?", r.AnchorId).Update("live_permission", liveModel.AnchorPermissionDisabled).Error)
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, room, session := setupAccessTest(t)
			require.NoError(t, s.DB.Model(&liveModel.LiveAccount{}).Where("id = ?", 501).Updates(map[string]interface{}{"nickname": "用户昵称", "signature": "公开签名", "gender": 2, "country_code": "MY"}).Error)
			if tc.change != nil {
				tc.change(t, s, room, session)
			}
			profiles, err := (SocialDirectory{}).Profiles(context.Background(), s.DB, tc.viewer, []uint64{501, 602})
			require.NoError(t, err)
			require.Len(t, profiles, 2)
			u := profiles[501]
			require.Equal(t, "用户昵称", u.Nickname)
			require.Equal(t, "公开签名", u.Signature)
			require.Equal(t, tc.live, u.IsLive)
			if tc.live {
				require.Equal(t, room.RoomNo, u.RoomNo)
				require.Equal(t, session.SessionNo, u.SessionNo)
			} else {
				require.Empty(t, u.RoomNo)
				require.Empty(t, u.SessionNo)
			}
			require.False(t, profiles[602].IsLive)
			raw, err := json.Marshal(u)
			require.NoError(t, err)
			var data map[string]interface{}
			require.NoError(t, json.Unmarshal(raw, &data))
			for _, key := range []string{"id", "userId", "phone", "password", "username", "tokenVersion", "realNameStatus", "birthday"} {
				require.NotContains(t, data, key)
			}
		})
	}
}

func TestSocialUserListsHistoryAndProfileUseSameAccount(t *testing.T) {
	s, room, _ := setupAccessTest(t)
	ctx := context.Background()
	require.NoError(t, s.DB.Model(&liveModel.LiveAccount{}).Where("id = ?", 501).Update("account_no", "10000501").Error)
	_, err := s.ChangeRelationship(ctx, 601, socialReq.Target{AccountNo: "10000501"}, socialService.Follow, "")
	require.NoError(t, err)
	following, err := s.ListUsers(ctx, 601, "following", socialReq.CursorSearch{}, 0)
	require.NoError(t, err)
	require.Len(t, following.List, 1)
	profile, err := (&AccountService{}).GetViewerPublicProfile(ctx, 601, "10000501")
	require.NoError(t, err)
	require.Equal(t, profile, following.List[0].User)
	online, err := s.FollowingOnline(ctx, 601, socialReq.CursorSearch{})
	require.NoError(t, err)
	require.Len(t, online.List, 1)
	require.Equal(t, profile, online.List[0].User)
	fans, err := s.ListUsers(ctx, 501, "followers", socialReq.CursorSearch{}, 0)
	require.NoError(t, err)
	require.Equal(t, "viewer", fans.List[0].AccountNo)
	r := socialReq.Resource{ResourceType: socialModel.ResourceLiveRoom, ResourceID: room.RoomNo}
	_, err = s.OpenHistory(ctx, 601, socialReq.HistoryOpen{Resource: r, RequestID: "profile-history-1"})
	require.NoError(t, err)
	history, err := s.HistoryList(ctx, 601, socialReq.CursorSearch{})
	require.NoError(t, err)
	require.Len(t, history.List, 1)
	require.Equal(t, profile, history.List[0].User)
	require.Equal(t, room.RoomNo, history.List[0].ResourceID)
	raw, err := json.Marshal(history.List[0])
	require.NoError(t, err)
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &data))
	for _, key := range []string{"ownerUserId", "title", "coverUrl", "sessionToken"} {
		require.NotContains(t, data, key)
	}
	other, err := s.HistoryList(ctx, 602, socialReq.CursorSearch{})
	require.NoError(t, err)
	require.Empty(t, other.List)
	_, err = s.ChangeRelationship(ctx, 501, socialReq.Target{AccountNo: "viewer"}, socialService.Block, "")
	require.NoError(t, err)
	history, err = s.HistoryList(ctx, 601, socialReq.CursorSearch{})
	require.NoError(t, err)
	require.Empty(t, history.List)
	require.NoError(t, s.DeleteHistory(ctx, 601, &r))
}

func TestPublicProfileBatchQueryCountDoesNotGrowWithUsers(t *testing.T) {
	s, _, _ := setupAccessTest(t)
	ids := []uint64{501}
	for i := uint(1000); i < 1099; i++ {
		require.NoError(t, s.DB.Create(&liveModel.LiveAccount{GVA_MODEL: global.GVA_MODEL{ID: i}, AccountNo: fmt.Sprint(i), Username: fmt.Sprint(i), Status: liveModel.LiveAccountStatusNormal}).Error)
		ids = append(ids, uint64(i))
	}
	count := 0
	name := "test:profile_query_count"
	cb := func(*gorm.DB) { count++ }
	require.NoError(t, s.DB.Callback().Row().Before("gorm:row").Register(name, cb))
	t.Cleanup(func() { s.DB.Callback().Row().Remove(name) })
	profiles, err := (SocialDirectory{}).Profiles(context.Background(), s.DB, 601, ids)
	require.NoError(t, err)
	require.Len(t, profiles, 100)
	require.Equal(t, 2, count)
	count = 0
	_, err = (SocialDirectory{}).Profiles(context.Background(), s.DB, 601, []uint64{501})
	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.NoError(t, s.DB.Model(&liveModel.LiveAccount{}).Where("id = ?", 501).Update("status", liveModel.LiveAccountStatusBanned).Error)
	profiles, err = (SocialDirectory{}).Profiles(context.Background(), s.DB, 601, []uint64{501})
	require.NoError(t, err)
	require.Empty(t, profiles)
}
