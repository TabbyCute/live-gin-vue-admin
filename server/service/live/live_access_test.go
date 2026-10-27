package live

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"tb_live_module/config"
	"tb_live_module/global"
	liveModel "tb_live_module/model/live"
	socialModel "tb_live_module/model/social"
	socialReq "tb_live_module/model/social/request"
	socialService "tb_live_module/service/social"
)

func setupAccessTest(t *testing.T) (*socialService.Service, liveModel.LiveRoom, liveModel.LiveSession) {
	t.Helper()
	setupRoomTestDB(t)
	db := global.GVA_DB
	require.NoError(t, db.AutoMigrate(append(socialModel.Models(), &liveModel.LiveAccount{})...))
	for i, id := range []uint{501, 601, 602} {
		require.NoError(t, db.Create(&liveModel.LiveAccount{GVA_MODEL: global.GVA_MODEL{ID: id}, AccountNo: []string{"host", "viewer", "other"}[i], Username: []string{"host", "viewer", "other"}[i], Password: "hash", Status: liveModel.LiveAccountStatusNormal}).Error)
	}
	anchor, _ := createRoomTestAnchor(t)
	room := liveModel.LiveRoom{RoomNo: "secure-room", AnchorId: anchor.ID, StreamName: "secure_stream", Status: liveModel.LiveRoomStatusNormal, Visibility: liveModel.LiveRoomVisibilityFollowers, LiveStatus: liveModel.LiveRoomLiving}
	require.NoError(t, db.Create(&room).Error)
	session := liveModel.LiveSession{SessionNo: "session-secure", RoomId: room.ID, AnchorId: anchor.ID, Status: liveModel.LiveSessionLiving, StartedAt: time.Now().UnixMilli()}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Model(&room).Update("current_session_id", session.ID).Error)
	previous, previousConfig := socialService.Default, global.GVA_CONFIG.Social
	global.GVA_CONFIG.Social = config.Social{ProtectedPlaybackEnabled: true, MediaGatewayVerified: true, PlaybackBaseURL: "https://media.example.test", PlaybackSigningKey: strings.Repeat("test-key-", 5)}
	s := &socialService.Service{DB: db, Directory: SocialDirectory{}, Config: global.GVA_CONFIG.Social, Resources: map[socialModel.ResourceType]socialService.ResourceProvider{socialModel.ResourceLiveRoom: SocialRoomResource{}}}
	socialService.Default = s
	t.Cleanup(func() { socialService.Default = previous; global.GVA_CONFIG.Social = previousConfig })
	return s, room, session
}
func TestLiveAccessFollowsBlocksAndCopiedPlayback(t *testing.T) {
	s, room, _ := setupAccessTest(t)
	ctx := context.Background()
	access := &LiveAccessService{}
	_, err := access.CheckLiveAccess(ctx, 601, room.RoomNo, true)
	require.ErrorIs(t, err, ErrLiveAccessDenied)
	_, err = access.CheckLiveAccess(ctx, 0, room.RoomNo, true)
	require.ErrorIs(t, err, ErrLiveAccessDenied)
	_, err = s.ChangeRelationship(ctx, 601, socialReq.Target{AccountNo: "host"}, socialService.Follow, "")
	require.NoError(t, err)
	_, err = access.CheckLiveAccess(ctx, 601, room.RoomNo, true)
	require.NoError(t, err)
	playback, err := access.AuthorizePlayback(ctx, 601, room.RoomNo)
	require.NoError(t, err)
	u, err := url.Parse(playback.PlayURL)
	require.NoError(t, err)
	token := u.Query().Get("playbackToken")
	require.NoError(t, access.ValidatePlayback(ctx, 601, token, u.Path))
	require.Error(t, access.ValidatePlayback(ctx, 602, token, u.Path))
	for _, path := range []string{"/live/other.m3u8", "/live/../secure_stream.m3u8", "/live/secure_stream.flv", "/live/secure_stream-12.ts/evil", "/wrong/secure_stream.m3u8"} {
		require.Error(t, access.ValidatePlayback(ctx, 601, token, path))
	}
	require.NoError(t, access.ValidatePlayback(ctx, 601, token, "/live/secure_stream-12.ts"))
	_, err = s.ChangeRelationship(ctx, 601, socialReq.Target{AccountNo: "host"}, socialService.Unfollow, "")
	require.NoError(t, err)
	require.Error(t, access.ValidatePlayback(ctx, 601, token, u.Path))
	_, err = s.ChangeRelationship(ctx, 601, socialReq.Target{AccountNo: "host"}, socialService.Follow, "")
	require.NoError(t, err)
	_, err = s.ChangeRelationship(ctx, 501, socialReq.Target{AccountNo: "viewer"}, socialService.Block, "")
	require.NoError(t, err)
	require.Error(t, access.ValidatePlayback(ctx, 601, token, u.Path))
	// URL签名不能替代当前数据库关系，也不能突破公开房间的双向拉黑。
	require.NoError(t, s.DB.Model(&room).Update("visibility", liveModel.LiveRoomVisibilityPublic).Error)
	_, err = access.CheckLiveAccess(ctx, 601, room.RoomNo, true)
	require.Error(t, err)
}
func TestLiveAccessClosedPrivateDisabledAndDBFailure(t *testing.T) {
	s, room, session := setupAccessTest(t)
	ctx := context.Background()
	access := &LiveAccessService{}
	require.NoError(t, s.DB.Model(&room).Update("visibility", liveModel.LiveRoomVisibilityPrivate).Error)
	_, err := access.CheckLiveAccess(ctx, 501, room.RoomNo, true)
	require.NoError(t, err)
	_, err = access.CheckLiveAccess(ctx, 601, room.RoomNo, true)
	require.Error(t, err)
	require.NoError(t, s.DB.Model(&session).Update("status", liveModel.LiveSessionEnded).Error)
	_, err = access.CheckLiveAccess(ctx, 501, room.RoomNo, true)
	require.Error(t, err)
	require.NoError(t, s.DB.Model(&session).Update("status", liveModel.LiveSessionLiving).Error)
	require.NoError(t, s.DB.Model(&liveModel.LiveAccount{}).Where("id=501").Update("status", liveModel.LiveAccountStatusBanned).Error)
	_, err = access.CheckLiveAccess(ctx, 501, room.RoomNo, true)
	require.Error(t, err)
	previous := global.GVA_DB
	global.GVA_DB = nil
	_, err = access.CheckLiveAccess(ctx, 601, room.RoomNo, true)
	require.Error(t, err)
	global.GVA_DB = previous
}
func TestProtectedRoomGate(t *testing.T) {
	previous := global.GVA_CONFIG.Social
	global.GVA_CONFIG.Social = config.Social{}
	defer func() { global.GVA_CONFIG.Social = previous }()
	public, followers, private := liveModel.LiveRoomVisibilityPublic, liveModel.LiveRoomVisibilityFollowers, liveModel.LiveRoomVisibilityPrivate
	require.NoError(t, requireProtectedMedia(&public))
	require.ErrorIs(t, requireProtectedMedia(&followers), ErrMediaGatewayRequired)
	require.ErrorIs(t, requireProtectedMedia(&private), ErrMediaGatewayRequired)
}
func TestSocialRoomResourceChecksAccess(t *testing.T) {
	s, room, _ := setupAccessTest(t)
	ctx := context.Background()
	r := socialReq.Resource{ResourceType: socialModel.ResourceLiveRoom, ResourceID: room.RoomNo}
	_, err := s.Favorite(ctx, 601, r, true)
	require.ErrorIs(t, err, socialService.ErrResource)
	_, err = s.ChangeRelationship(ctx, 601, socialReq.Target{AccountNo: "host"}, socialService.Follow, "")
	require.NoError(t, err)
	_, err = s.Favorite(ctx, 601, r, true)
	require.NoError(t, err)
	_, err = s.ChangeRelationship(ctx, 501, socialReq.Target{AccountNo: "viewer"}, socialService.Block, "")
	require.NoError(t, err)
	list, err := s.FavoriteStatus(ctx, 601, []socialReq.Resource{r})
	require.NoError(t, err)
	require.False(t, list[0].Favorite)
}
