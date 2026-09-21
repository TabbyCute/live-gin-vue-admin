package live

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"tb_live_module/global"
	liveModel "tb_live_module/model/live"
	liveReq "tb_live_module/model/live/request"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAnchorTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&liveModel.LiveAnchor{}, &liveModel.LiveCategory{}))
	previousDB := global.GVA_DB
	global.GVA_DB = db
	t.Cleanup(func() { global.GVA_DB = previousDB })
}

func uint8Ptr(value uint8) *uint8 { return &value }

func TestAnchorApplyReapplyAndAnchorNo(t *testing.T) {
	setupAnchorTestDB(t)
	service := AnchorService{}

	anchor, err := service.ApplyAnchor(11, liveReq.AnchorApplyReq{
		Nickname: "Alice", ChannelId: 1, TagIds: []uint64{1, 3},
	})
	require.NoError(t, err)
	require.Equal(t, "1100001", anchor.AnchorNo)
	require.Equal(t, liveModel.AnchorApplyStatusPending, anchor.ApplyStatus)

	_, err = service.ApplyAnchor(11, liveReq.AnchorApplyReq{Nickname: "Alice", ChannelId: 1})
	require.ErrorIs(t, err, ErrAnchorApplyPending)

	require.NoError(t, global.GVA_DB.Model(&liveModel.LiveAnchor{}).Where("id = ?", anchor.ID).Updates(map[string]interface{}{
		"apply_status":  liveModel.AnchorApplyStatusRejected,
		"audit_at":      int64(123),
		"audit_user_id": uint64(9),
		"reject_reason": "资料不完整",
	}).Error)
	reapplied, err := service.ApplyAnchor(11, liveReq.AnchorApplyReq{
		Nickname: "Alice 2", ChannelId: 99, TagIds: []uint64{},
	})
	require.NoError(t, err)
	require.Equal(t, anchor.ID, reapplied.ID)
	require.Equal(t, "1100001", reapplied.AnchorNo, "重新申请不能改写原渠道生成的主播编号")
	require.Equal(t, uint64(1), reapplied.ChannelId)
	require.Zero(t, reapplied.AuditAt)
	require.Zero(t, reapplied.AuditUserId)
	require.Empty(t, reapplied.RejectReason)
	require.Equal(t, "Alice 2", reapplied.Nickname)
}

func TestAnchorClientResponsesDoNotExposePrimaryKey(t *testing.T) {
	setupAnchorTestDB(t)
	service := AnchorService{}
	anchor := liveModel.LiveAnchor{
		UserId: 21, AnchorNo: "3100001", Nickname: "Public",
		ApplyStatus: liveModel.AnchorApplyStatusApproved,
		Status:      liveModel.AnchorStatusNormal,
	}
	require.NoError(t, global.GVA_DB.Create(&anchor).Error)

	myInfo, err := service.GetMyAnchorInfo(anchor.UserId)
	require.NoError(t, err)
	applyStatus, err := service.GetApplyStatus(anchor.UserId)
	require.NoError(t, err)
	publicDetail, err := service.GetAnchorPublicDetail(anchor.AnchorNo)
	require.NoError(t, err)
	require.Equal(t, anchor.AnchorNo, publicDetail.AnchorNo)

	clientResponses := map[string]interface{}{
		"my anchor info": myInfo,
		"apply status":   applyStatus,
		"public detail":  publicDetail,
	}
	for name, payload := range clientResponses {
		raw, marshalErr := json.Marshal(payload)
		require.NoError(t, marshalErr)
		require.NotContains(t, string(raw), `"id"`, "%s 不得返回 LiveAnchor 主键", name)
		require.NotContains(t, string(raw), `"anchorId"`, "%s 不得返回 LiveAnchor 主键", name)
	}

	_, err = service.GetAnchorPublicDetail("not-exists")
	require.ErrorIs(t, err, ErrAnchorNotFound)
}

func TestAnchorProfileWhitelistUpdatesZeroValues(t *testing.T) {
	setupAnchorTestDB(t)
	service := AnchorService{}
	require.NoError(t, global.GVA_DB.Create(&liveModel.LiveCategory{
		GVA_MODEL: global.GVA_MODEL{ID: 8}, Code: "talk", Name: "聊天", Status: liveModel.LiveCategoryStatusEnabled,
	}).Error)
	birthday := "1998-01-01"
	anchor, err := service.ApplyAnchor(12, liveReq.AnchorApplyReq{
		Nickname: "Before", ChannelId: 2, Gender: 2, Birthday: &birthday,
		CategoryId: 8, TagIds: []uint64{1, 2}, Signature: "before",
	})
	require.NoError(t, err)

	err = service.UpdateMyAnchorProfile(12, liveReq.AnchorProfileUpdateReq{
		Nickname: "After", Gender: 0, Birthday: nil, CategoryId: 0, TagIds: []uint64{}, Signature: "",
	})
	require.NoError(t, err)
	var refreshed liveModel.LiveAnchor
	require.NoError(t, global.GVA_DB.First(&refreshed, anchor.ID).Error)
	require.Equal(t, uint8(0), refreshed.Gender)
	require.Nil(t, refreshed.Birthday)
	require.Zero(t, refreshed.CategoryId)
	require.Empty(t, refreshed.TagIds)
	require.Empty(t, refreshed.Signature)
	require.Equal(t, "After", refreshed.Nickname)
	require.Equal(t, liveModel.AnchorApplyStatusPending, refreshed.ApplyStatus, "资料接口不能越权修改审核状态")
}

func TestAnchorAuditStateMachineAndPermissionSeparation(t *testing.T) {
	setupAnchorTestDB(t)
	service := AnchorService{}
	anchor, err := service.ApplyAnchor(13, liveReq.AnchorApplyReq{Nickname: "Audit", ChannelId: 1})
	require.NoError(t, err)

	err = service.AuditAnchor(888, liveReq.AnchorAuditReq{AnchorId: anchor.ID, ApplyStatus: liveModel.AnchorApplyStatusApproved})
	require.NoError(t, err)
	var refreshed liveModel.LiveAnchor
	require.NoError(t, global.GVA_DB.First(&refreshed, anchor.ID).Error)
	require.Equal(t, liveModel.AnchorApplyStatusApproved, refreshed.ApplyStatus)
	require.Equal(t, uint64(888), refreshed.AuditUserId)
	require.Equal(t, liveModel.AnchorPermissionDisabled, refreshed.LivePermission, "审核与直播权限保持分离")

	err = service.AuditAnchor(888, liveReq.AnchorAuditReq{AnchorId: anchor.ID, ApplyStatus: liveModel.AnchorApplyStatusRejected, RejectReason: "late"})
	require.ErrorIs(t, err, ErrInvalidApplyStatus)
}

func TestAnchorLiveAndPkChecks(t *testing.T) {
	setupAnchorTestDB(t)
	service := AnchorService{}
	anchor := liveModel.LiveAnchor{
		UserId: 14, AnchorNo: "1100001", Nickname: "Live",
		ApplyStatus:         liveModel.AnchorApplyStatusApproved,
		Status:              liveModel.AnchorStatusNormal,
		LivePermission:      liveModel.AnchorPermissionEnabled,
		PkPermission:        liveModel.AnchorPermissionDisabled,
		RecommendPermission: liveModel.AnchorPermissionEnabled,
	}
	require.NoError(t, global.GVA_DB.Create(&anchor).Error)

	result, err := service.CheckLivePermission(14)
	require.NoError(t, err)
	require.True(t, result.Allow)

	result, err = service.CheckPkPermission(14)
	require.NoError(t, err)
	require.False(t, result.Allow)
	require.Equal(t, "PK_PERMISSION_DISABLED", result.Code)

	require.NoError(t, global.GVA_DB.Model(&anchor).Updates(map[string]interface{}{
		"status": liveModel.AnchorStatusBanned, "ban_until": time.Now().Add(-time.Minute).UnixMilli(),
	}).Error)
	result, err = service.CheckLivePermission(14)
	require.NoError(t, err)
	require.False(t, result.Allow)
	require.Equal(t, "ANCHOR_BANNED", result.Code)
	var status uint8
	require.NoError(t, global.GVA_DB.Model(&anchor).Select("status").Scan(&status).Error)
	require.Equal(t, liveModel.AnchorStatusBanned, status, "GET check 语义不能隐式恢复数据库状态")
}

func TestAnchorPermissionRecommendAgencyAndCertValidation(t *testing.T) {
	setupAnchorTestDB(t)
	service := AnchorService{}
	anchor := liveModel.LiveAnchor{
		UserId: 15, AnchorNo: "1100001", Nickname: "Admin",
		Status: liveModel.AnchorStatusNormal, RecommendPermission: liveModel.AnchorPermissionDisabled,
	}
	require.NoError(t, global.GVA_DB.Create(&anchor).Error)
	require.NoError(t, global.GVA_DB.Model(&anchor).Update("recommend_permission", liveModel.AnchorPermissionDisabled).Error)

	err := service.UpdateAnchorPermission(liveReq.AnchorPermissionUpdateReq{
		AnchorId: anchor.ID, LivePermission: uint8Ptr(0), PkPermission: uint8Ptr(1),
		RecommendPermission: uint8Ptr(1), WithdrawPermission: uint8Ptr(0),
	})
	require.ErrorIs(t, err, ErrInvalidPermission)

	err = service.UpdateAnchorRecommend(liveReq.AnchorRecommendUpdateReq{
		AnchorId: anchor.ID, IsRecommended: uint8Ptr(1),
	})
	require.ErrorIs(t, err, ErrRecommendPermissionClosed)

	err = service.UpdateAnchorAgency(liveReq.AnchorAgencyUpdateReq{AnchorId: anchor.ID, AgencyId: 77})
	require.NoError(t, err)
	err = service.UpdateAnchorAgency(liveReq.AnchorAgencyUpdateReq{AnchorId: anchor.ID, AgencyId: 0})
	require.NoError(t, err)
	var refreshed liveModel.LiveAnchor
	require.NoError(t, global.GVA_DB.First(&refreshed, anchor.ID).Error)
	require.Zero(t, refreshed.AgencyId)
	require.Zero(t, refreshed.AgencyJoinAt)

	err = service.UpdateAnchorCert(liveReq.AnchorCertUpdateReq{
		AnchorId: anchor.ID, CertStatus: uint8Ptr(liveModel.AnchorCertStatusApproved), CertType: uint8Ptr(0), CertName: "官方主播",
	})
	require.True(t, errors.Is(err, ErrInvalidCertState))
	err = service.UpdateAnchorCert(liveReq.AnchorCertUpdateReq{
		AnchorId: anchor.ID, CertStatus: uint8Ptr(liveModel.AnchorCertStatusApproved),
		CertType: uint8Ptr(liveModel.AnchorCertTypeOfficial), CertName: "官方主播",
	})
	require.NoError(t, err)
}

func TestCancelledAnchorCannotBeRestored(t *testing.T) {
	setupAnchorTestDB(t)
	service := AnchorService{}
	anchor := liveModel.LiveAnchor{UserId: 16, AnchorNo: "1100001", Nickname: "Cancelled", Status: liveModel.AnchorStatusCancelled}
	require.NoError(t, global.GVA_DB.Create(&anchor).Error)
	err := service.UpdateAnchorStatus(liveReq.AnchorStatusUpdateReq{AnchorId: anchor.ID, Status: uint8Ptr(liveModel.AnchorStatusNormal)})
	require.ErrorIs(t, err, ErrCancelledCannotRestore)
}
