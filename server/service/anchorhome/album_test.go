package anchorhome

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/glebarez/sqlite"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"tb_live_module/global"
	model "tb_live_module/model/anchorhome"
	req "tb_live_module/model/anchorhome/request"
	resp "tb_live_module/model/anchorhome/response"
	"tb_live_module/model/live"
	"tb_live_module/model/social"
	"tb_live_module/model/system"
	"testing"
	"time"
)

func albumFixture(t *testing.T, mySQL bool) *AlbumService {
	t.Helper()
	var db *gorm.DB
	var err error
	if mySQL {
		raw := os.Getenv("ANCHOR_HOME_ALBUM_MYSQL_DSN")
		if raw == "" {
			t.Skip("guarded local ANCHOR_HOME_ALBUM_MYSQL_DSN not set")
		}
		cfg, e := driver.ParseDSN(raw)
		require.NoError(t, e)
		require.Equal(t, "tcp", cfg.Net)
		require.Equal(t, "127.0.0.1:13339", cfg.Addr)
		require.Equal(t, "anchor_home_test_album", cfg.DBName)
		cfg.ParseTime = true
		db, err = gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true, Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		var tables []string
		require.NoError(t, db.Raw("SHOW TABLES").Scan(&tables).Error)
		for _, table := range tables {
			require.Regexp(t, `^[a-z_]+$`, table)
			require.NoError(t, db.Exec("DROP TABLE `"+table+"`").Error)
		}
	} else {
		db, err = gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "album.db")+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true, Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
	}
	pool, err := db.DB()
	require.NoError(t, err)
	if mySQL {
		pool.SetMaxOpenConns(12)
	} else {
		pool.SetMaxOpenConns(1)
	}
	t.Cleanup(func() { _ = pool.Close() })
	require.NoError(t, db.AutoMigrate(&live.LiveAccount{}, &live.LiveAnchor{}, &system.SysUser{}, &system.SysAuthority{}, &system.SysUserAuthority{}, &social.SocialBlock{}))
	require.NoError(t, MigrateAlbumSchema(db))
	for _, id := range []uint{10, 20, 30} {
		require.NoError(t, db.Create(&live.LiveAccount{GVA_MODEL: global.GVA_MODEL{ID: id}, AccountNo: fmt.Sprintf("100000%d", id), Username: fmt.Sprintf("album%d", id), Password: "test-only-unused", Status: 1}).Error)
	}
	require.NoError(t, db.Create(&live.LiveAnchor{GVA_MODEL: global.GVA_MODEL{ID: 10}, UserId: 10, AnchorNo: "album_anchor", ApplyStatus: 2, Status: 1}).Error)
	require.NoError(t, db.Create(&live.LiveAnchor{GVA_MODEL: global.GVA_MODEL{ID: 20}, UserId: 20, AnchorNo: "album_other", ApplyStatus: 2, Status: 1}).Error)
	require.NoError(t, db.Create(&system.SysAuthority{AuthorityId: 888, AuthorityName: "Album Moderator"}).Error)
	for _, id := range []uint{1, 2, 3} {
		require.NoError(t, db.Create(&system.SysUser{GVA_MODEL: global.GVA_MODEL{ID: id}, Username: fmt.Sprintf("moderator%d", id), Enable: 1, AuthorityId: 888}).Error)
	}
	require.NoError(t, db.Create(&system.SysUserAuthority{SysUserId: 2, SysAuthorityAuthorityId: 888}).Error)
	s, err := NewAlbumService(db, AlbumOptions{StorageDir: filepath.Join(t.TempDir(), "media")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}
func imageBytes(t testing.TB) []byte {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 120, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 120; x++ {
			im.Set(x, y, color.RGBA{R: uint8(x * 2), G: uint8(y * 3), B: 50, A: 255})
		}
	}
	var b bytes.Buffer
	require.NoError(t, png.Encode(&b, im))
	return b.Bytes()
}
func uploadForTest(t *testing.T, s *AlbumService, version uint64, key string) resp.AlbumResult {
	t.Helper()
	v, err := s.Upload(context.Background(), 10, req.AlbumMutation{RequestID: key, Version: version}, bytes.NewReader(imageBytes(t)))
	require.NoError(t, err)
	return v
}
func reviewForTest(t *testing.T, s *AlbumService, p resp.AlbumResult, key, decision string) resp.AlbumResult {
	t.Helper()
	v, err := s.Review(context.Background(), 2, req.ReviewPhoto{AlbumMutation: req.AlbumMutation{RequestID: key, Version: p.PhotoVersion}, PhotoNo: p.PhotoNo, Decision: decision, Reason: "本地审核测试"})
	require.NoError(t, err)
	return v
}
func assertNoRead(t *testing.T, s *AlbumService, viewer uint64, kind, no string) {
	t.Helper()
	f, _, err := s.ReadPhoto(context.Background(), viewer, kind, no, "full")
	if f != nil {
		f.Close()
	}
	require.Error(t, err)
}

func TestAlbumLifecycleAndIdempotency(t *testing.T) {
	s := albumFixture(t, false)
	ctx := context.Background()
	album, err := s.OwnerAlbum(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, uint64(0), album.Version)
	require.Empty(t, album.List)
	p := uploadForTest(t, s, 0, "upload_first")
	require.Equal(t, uint64(1), p.Version)
	assertNoRead(t, s, 0, "public", p.PhotoNo)
	duplicate, err := s.Upload(ctx, 10, req.AlbumMutation{RequestID: "upload_first", Version: 0}, bytes.NewReader(imageBytes(t)))
	require.NoError(t, err)
	require.Equal(t, p, duplicate)
	_, err = s.Delete(ctx, 10, req.DeletePhoto{AlbumMutation: req.AlbumMutation{RequestID: "upload_first", Version: 1}, PhotoNo: p.PhotoNo})
	require.ErrorIs(t, err, ErrIdempotency)
	f, _, err := s.ReadPhoto(ctx, 10, "owner", p.PhotoNo, "full")
	require.NoError(t, err)
	raw, err := io.ReadAll(f)
	f.Close()
	require.NoError(t, err)
	_, format, err := image.Decode(bytes.NewReader(raw))
	require.NoError(t, err)
	require.Equal(t, "jpeg", format)
	p = reviewForTest(t, s, p, "review_first", "approve")
	photos, err := s.ApprovedPhotos(ctx, 0, "album_anchor")
	require.NoError(t, err)
	require.Len(t, photos, 1)
	f, _, err = s.ReadPhoto(ctx, 30, "public", p.PhotoNo, "thumb")
	require.NoError(t, err)
	f.Close()
	replay, err := s.Review(ctx, 2, req.ReviewPhoto{AlbumMutation: req.AlbumMutation{RequestID: "review_first", Version: 1}, PhotoNo: p.PhotoNo, Decision: "approve", Reason: "本地审核测试"})
	require.NoError(t, err)
	require.Equal(t, p, replay)
	p = reviewForTest(t, s, p, "remove_first", "takedown")
	assertNoRead(t, s, 0, "public", p.PhotoNo)
	_, err = s.Review(ctx, 2, req.ReviewPhoto{AlbumMutation: req.AlbumMutation{RequestID: "reapprove_fail", Version: p.PhotoVersion}, PhotoNo: p.PhotoNo, Decision: "approve"})
	require.ErrorIs(t, err, ErrVersion)
	del := req.DeletePhoto{AlbumMutation: req.AlbumMutation{RequestID: "delete_first", Version: p.Version}, PhotoNo: p.PhotoNo}
	deleted, err := s.Delete(ctx, 10, del)
	require.NoError(t, err)
	again, err := s.Delete(ctx, 10, del)
	require.NoError(t, err)
	require.Equal(t, deleted, again)
	assertNoRead(t, s, 10, "owner", p.PhotoNo)
	assertNoRead(t, s, 2, "admin", p.PhotoNo)
	album, err = s.OwnerAlbum(ctx, 10)
	require.NoError(t, err)
	require.Empty(t, album.List)
	require.Equal(t, deleted.Version, album.Version)
	var audits []model.Audit
	require.NoError(t, s.DB.Order("id").Find(&audits).Error)
	require.Len(t, audits, 4)
	require.Equal(t, model.PhotoRemoved, audits[3].FromStatus)
	require.Equal(t, model.PhotoDeleted, audits[3].ToStatus)
	require.Error(t, s.DB.Model(&model.Audit{}).Where("id=?", audits[0].ID).Update("reason", "tamper").Error)
	require.Error(t, s.DB.Where("id=?", audits[0].ID).Delete(&model.Audit{}).Error)
	require.Error(t, s.DB.Exec("UPDATE anchor_home_command SET result='{}'").Error)
}

func TestAlbumAuthorizationAndRevocation(t *testing.T) {
	s := albumFixture(t, false)
	ctx := context.Background()
	p := uploadForTest(t, s, 0, "upload_auth")
	assertNoRead(t, s, 20, "owner", p.PhotoNo)
	assertNoRead(t, s, 3, "admin", p.PhotoNo)
	_, err := s.AdminList(ctx, 3, req.AlbumSearch{})
	require.ErrorIs(t, err, ErrForbidden)
	_, err = s.Review(ctx, 3, req.ReviewPhoto{AlbumMutation: req.AlbumMutation{RequestID: "review_intruder", Version: 1}, PhotoNo: p.PhotoNo, Decision: "approve"})
	require.ErrorIs(t, err, ErrForbidden)
	p = reviewForTest(t, s, p, "approve_auth", "approve")
	for _, pair := range [][2]uint64{{10, 30}, {30, 10}} {
		block := social.SocialBlock{UserID: pair[0], TargetUserID: pair[1]}
		require.NoError(t, s.DB.Create(&block).Error)
		assertNoRead(t, s, 30, "public", p.PhotoNo)
		_, err = s.ApprovedPhotos(ctx, 30, "album_anchor")
		require.ErrorIs(t, err, ErrNotFound)
		require.NoError(t, s.DB.Delete(&block).Error)
	}
	require.NoError(t, s.DB.Model(&live.LiveAccount{}).Where("id=10").Update("status", 2).Error)
	assertNoRead(t, s, 0, "public", p.PhotoNo)
	assertNoRead(t, s, 10, "owner", p.PhotoNo)
	require.NoError(t, s.DB.Model(&live.LiveAccount{}).Where("id=10").Update("status", 1).Error)
	require.NoError(t, s.DB.Model(&live.LiveAnchor{}).Where("id=10").Update("apply_status", 1).Error)
	assertNoRead(t, s, 0, "public", p.PhotoNo)
	require.NoError(t, s.DB.Model(&live.LiveAnchor{}).Where("id=10").Update("apply_status", 2).Error)
	require.NoError(t, s.DB.Model(&system.SysUser{}).Where("id=2").Update("enable", 2).Error)
	assertNoRead(t, s, 2, "admin", p.PhotoNo)
	f, _, err := s.ReadPhoto(ctx, 1, "admin", p.PhotoNo, "full")
	require.NoError(t, err)
	f.Close()
	require.NoError(t, s.DB.Model(&system.SysUser{}).Where("id=1").Update("enable", 2).Error)
	assertNoRead(t, s, 1, "admin", p.PhotoNo)
}

func TestAlbumQuotaOrderAndVersions(t *testing.T) {
	s := albumFixture(t, false)
	s.Options.MaxPhotos = 2
	ctx := context.Background()
	first := uploadForTest(t, s, 0, "first_quota")
	second := uploadForTest(t, s, first.Version, "second_quota")
	second = reviewForTest(t, s, second, "reject_quota", "reject")
	_, err := s.Upload(ctx, 10, req.AlbumMutation{RequestID: "over_quota", Version: second.Version}, bytes.NewReader(imageBytes(t)))
	require.ErrorIs(t, err, ErrQuota)
	_, err = s.Reorder(ctx, 10, req.ReorderPhotos{AlbumMutation: req.AlbumMutation{RequestID: "stale_reorder", Version: 1}, PhotoNos: []string{second.PhotoNo, first.PhotoNo}})
	require.ErrorIs(t, err, ErrVersion)
	_, err = s.Reorder(ctx, 10, req.ReorderPhotos{AlbumMutation: req.AlbumMutation{RequestID: "missing_reorder", Version: second.Version}, PhotoNos: []string{first.PhotoNo}})
	require.ErrorIs(t, err, ErrVersion)
	r := req.ReorderPhotos{AlbumMutation: req.AlbumMutation{RequestID: "valid_reorder", Version: second.Version}, PhotoNos: []string{second.PhotoNo, first.PhotoNo}}
	out, err := s.Reorder(ctx, 10, r)
	require.NoError(t, err)
	again, err := s.Reorder(ctx, 10, r)
	require.NoError(t, err)
	require.Equal(t, out, again)
	album, err := s.OwnerAlbum(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, second.PhotoNo, album.List[0].PhotoNo)
	_, err = s.Reorder(ctx, 20, req.ReorderPhotos{AlbumMutation: req.AlbumMutation{RequestID: "foreign_reorder", Version: 0}, PhotoNos: []string{first.PhotoNo}})
	require.ErrorIs(t, err, ErrVersion)
	_, err = s.Delete(ctx, 20, req.DeletePhoto{AlbumMutation: req.AlbumMutation{RequestID: "foreign_delete", Version: 0}, PhotoNo: first.PhotoNo})
	require.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, s.DB.Model(&model.Album{}).Where("owner_id=10").Update("version", MaxSafeVersion).Error)
	_, err = s.Delete(ctx, 10, req.DeletePhoto{AlbumMutation: req.AlbumMutation{RequestID: "overflow_delete", Version: MaxSafeVersion}, PhotoNo: first.PhotoNo})
	require.ErrorIs(t, err, ErrInvalid)
}

func TestAlbumMediaValidationAndCleanup(t *testing.T) {
	s := albumFixture(t, false)
	ctx := context.Background()
	for i, b := range [][]byte{nil, []byte(`<svg onload="alert(1)"></svg>`), []byte("http://169.254.169.254/"), bytes.Repeat([]byte("a"), int(MaxUploadBytes+1)), []byte("GIF89a")} {
		_, err := s.Upload(ctx, 10, req.AlbumMutation{RequestID: fmt.Sprintf("invalid_%d", i)}, bytes.NewReader(b))
		require.ErrorIs(t, err, ErrInvalid)
	}
	_, err := NewAlbumService(s.DB, AlbumOptions{StorageDir: t.TempDir(), MaxPhotos: 31})
	require.ErrorIs(t, err, ErrInvalid)
	// A forged PNG IHDR exceeding the pixel budget is rejected before full decode.
	b := imageBytes(t)
	copy(b[16:20], []byte{0, 0, 0x2e, 0xe0})
	copy(b[20:24], []byte{0, 0, 0x2e, 0xe0})
	binary.BigEndian.PutUint32(b[29:33], crc32.ChecksumIEEE(b[12:29]))
	cfg, _, decodeErr := image.DecodeConfig(bytes.NewReader(b))
	require.NoError(t, decodeErr)
	require.Equal(t, 12000, cfg.Width)
	_, err = s.prepare(ctx, b)
	require.Error(t, err)
	p := uploadForTest(t, s, 0, "cleanup_upload")
	assertNoRead(t, s, 10, "owner", "../../etc/passwd")
	var photo model.Photo
	require.NoError(t, s.DB.Where("photo_no=?", p.PhotoNo).Take(&photo).Error)
	marker := "unrelated.txt"
	require.NoError(t, os.WriteFile(filepath.Join(s.Options.StorageDir, marker), []byte("preserve"), 0600))
	orphanKey := strings.Repeat("a", 32)
	orphan := filepath.Join(s.Options.StorageDir, orphanKey+".full.jpg")
	require.NoError(t, os.WriteFile(orphan, []byte("orphan"), 0600))
	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(orphan, old, old))
	count, err := s.CleanupOrphans(ctx, time.Now().Add(-24*time.Hour))
	require.NoError(t, err)
	require.Equal(t, 1, count)
	_, err = os.Stat(orphan)
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(s.Options.StorageDir, marker))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(s.Options.StorageDir, photo.StorageKey+".full.jpg"))
	require.NoError(t, err)
	outside := filepath.Join(t.TempDir(), "private.txt")
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0600))
	path := filepath.Join(s.Options.StorageDir, photo.StorageKey+".full.jpg")
	require.NoError(t, os.Remove(path))
	require.NoError(t, os.Symlink(outside, path))
	assertNoRead(t, s, 10, "owner", p.PhotoNo)
}

func TestAlbumWebPAndAdminBoundaries(t *testing.T) {
	s := albumFixture(t, false)
	ctx := context.Background()
	webp, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	require.NoError(t, err)
	photo, err := s.Upload(ctx, 10, req.AlbumMutation{RequestID: "webp_upload"}, bytes.NewReader(webp))
	require.NoError(t, err)
	page, err := s.AdminList(ctx, 2, req.AlbumSearch{Status: "pending", AnchorNo: "album_anchor"})
	require.NoError(t, err)
	require.Equal(t, int64(1), page.Total)
	require.Len(t, page.List, 1)
	require.Equal(t, photo.PhotoNo, page.List[0].PhotoNo)
	require.Equal(t, 1, page.Page)
	for _, q := range []req.AlbumSearch{{Page: 1001}, {PageSize: 101}, {Status: "unknown"}, {AnchorNo: "' OR 1=1"}} {
		_, err := s.AdminList(ctx, 2, q)
		require.ErrorIs(t, err, ErrInvalid)
	}
	for _, reason := range []string{"", strings.Repeat("中", 201), "line\nnewline"} {
		_, err := s.Review(ctx, 2, req.ReviewPhoto{AlbumMutation: req.AlbumMutation{RequestID: "invalid_reason", Version: 1}, PhotoNo: photo.PhotoNo, Decision: "reject", Reason: reason})
		require.ErrorIs(t, err, ErrInvalid)
	}
	_, _, err = s.ReadPhoto(ctx, 10, "owner", photo.PhotoNo, "../../full")
	require.ErrorIs(t, err, ErrInvalid)
}

func TestAlbumImageOrientationAndMetadata(t *testing.T) {
	im := image.NewRGBA(image.Rect(0, 0, 3, 2))
	im.Set(0, 0, color.RGBA{255, 0, 0, 255})
	for orientation := 1; orientation <= 8; orientation++ {
		out := orient(im, orientation)
		if orientation >= 5 {
			require.Equal(t, 2, out.Bounds().Dx())
			require.Equal(t, 3, out.Bounds().Dy())
		}
	}
	var b bytes.Buffer
	require.NoError(t, jpeg.Encode(&b, im, nil))
	raw := b.Bytes()
	segment := []byte{'E', 'x', 'i', 'f', 0, 0, 'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0}
	header := []byte{0xff, 0xe1, 0, 0}
	binary.BigEndian.PutUint16(header[2:], uint16(len(segment)+2))
	withExif := append(append(append([]byte{}, raw[:2]...), append(header, segment...)...), raw[2:]...)
	require.Equal(t, 6, jpegOrientation(withExif))
	s := albumFixture(t, false)
	prepared, err := s.prepare(context.Background(), withExif)
	require.NoError(t, err)
	require.Equal(t, 2, prepared.width)
	require.Equal(t, 3, prepared.height)
	clean, err := os.ReadFile(filepath.Join(s.Options.StorageDir, prepared.key+".full.jpg"))
	require.NoError(t, err)
	require.NotContains(t, string(clean), "Exif")
	for i := 0; i < len(withExif); i++ {
		_ = jpegOrientation(withExif[:i])
	}
}

func TestAlbumSchemaDetectsTampering(t *testing.T) {
	s := albumFixture(t, false)
	require.NoError(t, VerifyAlbumSchema(s.DB))
	require.NoError(t, s.DB.Exec("DROP TRIGGER trg_anchor_home_audit_update").Error)
	require.Error(t, VerifyAlbumSchema(s.DB))
	require.NoError(t, MigrateAlbumSchema(s.DB))
	require.NoError(t, s.DB.Exec("DROP INDEX uk_anchor_home_command").Error)
	require.Error(t, VerifyAlbumSchema(s.DB))
}

func TestAlbumMySQLConcurrentMutations(t *testing.T) {
	s := albumFixture(t, true)
	ctx := context.Background()
	first := uploadForTest(t, s, 0, "mysql_first")
	second := uploadForTest(t, s, first.Version, "mysql_second")
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	results := make(chan resp.AlbumResult, 16)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := s.Reorder(ctx, 10, req.ReorderPhotos{AlbumMutation: req.AlbumMutation{RequestID: "mysql_same_reorder", Version: second.Version}, PhotoNos: []string{second.PhotoNo, first.PhotoNo}})
			errs <- err
			results <- r
		}()
	}
	wg.Wait()
	close(errs)
	close(results)
	for err := range errs {
		require.NoError(t, err)
	}
	for r := range results {
		require.Equal(t, second.Version+1, r.Version)
	}
	// Concurrent reviewers of the same request serialize to one durable audit/result.
	errs = make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Review(ctx, 2, req.ReviewPhoto{AlbumMutation: req.AlbumMutation{RequestID: "mysql_same_review", Version: 1}, PhotoNo: first.PhotoNo, Decision: "approve"})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var audits int64
	require.NoError(t, s.DB.Model(&model.Audit{}).Where("request_id=?", "mysql_same_review").Count(&audits).Error)
	require.Equal(t, int64(1), audits)
	require.NoError(t, VerifyAlbumSchema(s.DB))
	// Different commands with the same CAS version cannot overwrite each other.
	album, err := s.OwnerAlbum(ctx, 10)
	require.NoError(t, err)
	type outcome struct {
		result resp.AlbumResult
		err    error
	}
	outcomes := make(chan outcome, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r, e := s.Reorder(ctx, 10, req.ReorderPhotos{AlbumMutation: req.AlbumMutation{RequestID: fmt.Sprintf("mysql_unique_%02d", i), Version: album.Version}, PhotoNos: []string{first.PhotoNo, second.PhotoNo}})
			outcomes <- outcome{r, e}
		}(i)
	}
	wg.Wait()
	close(outcomes)
	success := 0
	for o := range outcomes {
		if o.err == nil {
			success++
		} else {
			require.ErrorIs(t, o.err, ErrVersion)
		}
	}
	require.Equal(t, 1, success)
	// Same admin requestId on two different owners must yield one success and a
	// deterministic idempotency conflict, never a deadlock/duplicate side effect.
	other, err := s.Upload(ctx, 20, req.AlbumMutation{RequestID: "mysql_other_owner", Version: 0}, bytes.NewReader(imageBytes(t)))
	require.NoError(t, err)
	outcomes = make(chan outcome, 2)
	for _, p := range []resp.AlbumResult{second, other} {
		wg.Add(1)
		go func(p resp.AlbumResult) {
			defer wg.Done()
			r, e := s.Review(ctx, 2, req.ReviewPhoto{AlbumMutation: req.AlbumMutation{RequestID: "mysql_reuse_across_owners", Version: 1}, PhotoNo: p.PhotoNo, Decision: "approve"})
			outcomes <- outcome{r, e}
		}(p)
	}
	wg.Wait()
	close(outcomes)
	success = 0
	for o := range outcomes {
		if o.err == nil {
			success++
		} else {
			require.ErrorIs(t, o.err, ErrIdempotency)
		}
	}
	require.Equal(t, 1, success)
	// Owner delete and moderation share album lock order and cannot resurrect a photo.
	album, err = s.OwnerAlbum(ctx, 10)
	require.NoError(t, err)
	p := uploadForTest(t, s, album.Version, "mysql_delete_race_upload")
	outcomes = make(chan outcome, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		r, e := s.Delete(ctx, 10, req.DeletePhoto{AlbumMutation: req.AlbumMutation{RequestID: "mysql_delete_race", Version: p.Version}, PhotoNo: p.PhotoNo})
		outcomes <- outcome{r, e}
	}()
	go func() {
		defer wg.Done()
		r, e := s.Review(ctx, 2, req.ReviewPhoto{AlbumMutation: req.AlbumMutation{RequestID: "mysql_review_race", Version: 1}, PhotoNo: p.PhotoNo, Decision: "approve"})
		outcomes <- outcome{r, e}
	}()
	wg.Wait()
	close(outcomes)
	success = 0
	for o := range outcomes {
		if o.err == nil {
			success++
		} else {
			require.True(t, errors.Is(o.err, ErrVersion) || errors.Is(o.err, ErrNotFound), o.err)
		}
	}
	require.Equal(t, 1, success)
	// Structural drift fails verification before the API can start.
	require.NoError(t, s.DB.Exec("ALTER TABLE anchor_home_photo MODIFY reason varchar(799) NOT NULL DEFAULT ''").Error)
	require.Error(t, VerifyAlbumSchema(s.DB))
}

func TestAlbumUploadThrottleSurvivesDelete(t *testing.T) {
	s := albumFixture(t, false)
	s.Options.MaxUploadsPerDay = 1
	p := uploadForTest(t, s, 0, "throttle_upload")
	del, err := s.Delete(context.Background(), 10, req.DeletePhoto{AlbumMutation: req.AlbumMutation{RequestID: "throttle_delete", Version: p.Version}, PhotoNo: p.PhotoNo})
	require.NoError(t, err)
	_, err = s.Upload(context.Background(), 10, req.AlbumMutation{RequestID: "throttle_again", Version: del.Version}, bytes.NewReader(imageBytes(t)))
	require.ErrorIs(t, err, ErrUploadLimit)
}

type uncertainCommitPool struct{ *sql.DB }

func (p uncertainCommitPool) BeginTx(ctx context.Context, options *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := p.DB.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	return &uncertainCommitTx{tx}, nil
}

type uncertainCommitTx struct{ *sql.Tx }

func (t uncertainCommitTx) Commit() error {
	if err := t.Tx.Commit(); err != nil {
		return err
	}
	return errors.New("simulated connection lost after durable COMMIT")
}

func TestAlbumUploadUnknownCommitRetainsFilesAndReplays(t *testing.T) {
	s := albumFixture(t, false)
	pool, err := s.DB.DB()
	require.NoError(t, err)
	s.DB = s.DB.Session(&gorm.Session{NewDB: true})
	s.DB.Statement.ConnPool = uncertainCommitPool{pool}
	r := req.AlbumMutation{RequestID: "uncertain_upload", Version: 0}
	_, err = s.Upload(context.Background(), 10, r, bytes.NewReader(imageBytes(t)))
	require.Error(t, err)
	var photos []model.Photo
	require.NoError(t, s.DB.Find(&photos).Error)
	require.Len(t, photos, 1)
	for _, variant := range []string{"full", "thumb"} {
		_, err := os.Stat(filepath.Join(s.Options.StorageDir, photos[0].StorageKey+"."+variant+".jpg"))
		require.NoError(t, err)
	}
	result, err := s.Upload(context.Background(), 10, r, bytes.NewReader(imageBytes(t)))
	require.NoError(t, err)
	require.Equal(t, photos[0].PhotoNo, result.PhotoNo)
}

func TestAlbumUploadDefiniteRollbackRemovesOnlyOwnedFiles(t *testing.T) {
	s := albumFixture(t, false)
	marker := filepath.Join(s.Options.StorageDir, "keep.txt")
	require.NoError(t, os.WriteFile(marker, []byte("safe"), 0600))
	name := "album_test_fail_photo_insert"
	require.NoError(t, s.DB.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "anchor_home_photo" {
			_ = tx.AddError(errors.New("injected database write rejection"))
		}
	}))
	defer s.DB.Callback().Create().Remove(name)
	_, err := s.Upload(context.Background(), 10, req.AlbumMutation{RequestID: "rollback_upload", Version: 0}, bytes.NewReader(imageBytes(t)))
	require.Error(t, err)
	files, err := os.ReadDir(s.Options.StorageDir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, "keep.txt", files[0].Name())
}

func TestAlbumCleanupBoundedCursorAndRetention(t *testing.T) {
	s := albumFixture(t, false)
	ctx := context.Background()
	p := uploadForTest(t, s, 0, "retention_upload")
	_, err := s.Delete(ctx, 10, req.DeletePhoto{AlbumMutation: req.AlbumMutation{RequestID: "retention_delete", Version: p.Version}, PhotoNo: p.PhotoNo})
	require.NoError(t, err)
	var row model.Photo
	require.NoError(t, s.DB.Where("photo_no=?", p.PhotoNo).Take(&row).Error)
	old := time.Now().Add(-8 * 24 * time.Hour)
	require.NoError(t, s.DB.Model(&row).Update("updated_at", old.UnixMilli()).Error)
	for _, variant := range []string{"full", "thumb"} {
		require.NoError(t, os.Chtimes(filepath.Join(s.Options.StorageDir, row.StorageKey+"."+variant+".jpg"), old, old))
	}
	for i := 0; i < 1050; i++ {
		require.NoError(t, os.WriteFile(filepath.Join(s.Options.StorageDir, fmt.Sprintf("unrelated_%04d.txt", i)), nil, 0600))
	}
	count, err := s.CleanupOrphans(ctx, time.Now().Add(-24*time.Hour))
	require.NoError(t, err)
	require.NotNil(t, s.cleanupDir)
	more, err := s.CleanupOrphans(ctx, time.Now().Add(-24*time.Hour))
	require.NoError(t, err)
	require.Nil(t, s.cleanupDir)
	require.Equal(t, 2, count+more)
	var rows int64
	require.NoError(t, s.DB.Model(&model.Photo{}).Where("photo_no=?", p.PhotoNo).Count(&rows).Error)
	require.Equal(t, int64(1), rows)
}

func FuzzJPEGOrientation(f *testing.F) {
	f.Add([]byte{0xff, 0xd8, 0xff, 0xd9})
	f.Add([]byte("not-an-image"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<20 {
			return
		}
		v := jpegOrientation(b)
		if v < 1 || v > 8 {
			t.Fatalf("orientation out of range: %d", v)
		}
	})
}

func BenchmarkAlbumSanitizePhoto(b *testing.B) {
	im := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	for y := 0; y < 1080; y++ {
		for x := 0; x < 1920; x++ {
			im.SetRGBA(x, y, color.RGBA{uint8(x % 251), uint8(y % 241), uint8((x * y) % 239), 255})
		}
	}
	var raw bytes.Buffer
	require.NoError(b, jpeg.Encode(&raw, im, &jpeg.Options{Quality: 90}))
	data := raw.Bytes()
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	for b.Loop() {
		im, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			b.Fatal(err)
		}
		full := resized(im, 2048)
		if _, err = encodeJPEG(full); err != nil {
			b.Fatal(err)
		}
		if _, err = encodeJPEG(resized(full, 480)); err != nil {
			b.Fatal(err)
		}
	}
}
