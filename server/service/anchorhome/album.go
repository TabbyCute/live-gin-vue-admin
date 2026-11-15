package anchorhome

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"io"
	"math"
	"net/url"
	"os"
	"strings"
	"sync"
	model "tb_live_module/model/anchorhome"
	req "tb_live_module/model/anchorhome/request"
	resp "tb_live_module/model/anchorhome/response"
	"time"
	"unicode"
	"unicode/utf8"
)

const MaxSafeVersion uint64 = 9007199254740991
const MaxUploadBytes int64 = 8 << 20

type AlbumOptions struct {
	StorageDir       string
	PublicBase       string // trusted API prefix, normally /v1/app/anchor-home
	AdminBase        string
	MaxPhotos        int
	MaxUploadsPerDay int
	Concurrency      int
}
type AlbumService struct {
	DB         *gorm.DB
	Options    AlbumOptions
	root       *os.Root
	slots      chan struct{}
	mu         sync.Mutex
	owners     map[uint64]bool
	cleanupMu  sync.Mutex
	cleanupDir *os.File
}

var DefaultAlbum = &AlbumService{}

func NewAlbumService(db *gorm.DB, options AlbumOptions) (*AlbumService, error) {
	if db == nil || options.StorageDir == "" {
		return nil, ErrUnavailable
	}
	if options.MaxPhotos == 0 {
		options.MaxPhotos = 9
	}
	if options.MaxPhotos < 1 || options.MaxPhotos > 30 {
		return nil, ErrInvalid
	}
	if options.MaxUploadsPerDay == 0 {
		options.MaxUploadsPerDay = 50
	}
	if options.MaxUploadsPerDay < options.MaxPhotos || options.MaxUploadsPerDay > 500 {
		return nil, ErrInvalid
	}
	if options.Concurrency == 0 {
		options.Concurrency = 2
	}
	if options.Concurrency < 1 || options.Concurrency > 4 {
		return nil, ErrInvalid
	}
	if options.PublicBase == "" {
		options.PublicBase = "/v1/app/anchor-home"
	}
	if options.AdminBase == "" {
		options.AdminBase = "/v1/admin/anchor-home"
	}
	for _, base := range []string{options.PublicBase, options.AdminBase} {
		if !strings.HasPrefix(base, "/") || strings.HasPrefix(base, "//") || strings.ContainsAny(base, "?#\\") {
			return nil, ErrInvalid
		}
	}
	if err := os.MkdirAll(options.StorageDir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(options.StorageDir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, ErrUnavailable
	}
	root, err := os.OpenRoot(options.StorageDir)
	if err != nil {
		return nil, err
	}
	return &AlbumService{DB: db, Options: options, root: root, slots: make(chan struct{}, options.Concurrency), owners: map[uint64]bool{}}, nil
}
func (s *AlbumService) Close() error {
	s.cleanupMu.Lock()
	defer s.cleanupMu.Unlock()
	if s.cleanupDir != nil {
		_ = s.cleanupDir.Close()
		s.cleanupDir = nil
	}
	if s.root != nil {
		return s.root.Close()
	}
	return nil
}
func (s *AlbumService) ready() error {
	if s == nil || s.DB == nil || s.root == nil {
		return ErrUnavailable
	}
	return nil
}

func payloadHash(operation string, v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(append([]byte(operation+"\x00"), b...))
	return hex.EncodeToString(sum[:])
}
func validCommand(r req.AlbumMutation) bool {
	return len(r.RequestID) >= 8 && len(r.RequestID) <= 64 && opaque.MatchString(r.RequestID) && r.Version < MaxSafeVersion
}
func replay(db *gorm.DB, kind string, actor uint64, id, hash string) (resp.AlbumResult, bool, error) {
	var c model.Command
	err := db.Clauses(clause.Locking{Strength: "UPDATE"}).Where("actor_kind=? AND actor_id=? AND request_id=?", kind, actor, id).Take(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return resp.AlbumResult{}, false, nil
	}
	if err != nil {
		return resp.AlbumResult{}, false, err
	}
	if c.PayloadHash != hash {
		return resp.AlbumResult{}, false, ErrIdempotency
	}
	var result resp.AlbumResult
	err = json.Unmarshal([]byte(c.Result), &result)
	return result, true, err
}
func recordCommand(tx *gorm.DB, kind string, actor uint64, id, hash string, result resp.AlbumResult) error {
	b, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return tx.Create(&model.Command{ActorKind: kind, ActorID: actor, RequestID: id, PayloadHash: hash, Result: string(b), CreatedAt: time.Now().UnixMilli()}).Error
}
func lockAlbum(tx *gorm.DB, owner uint64) (model.Album, error) {
	a := model.Album{OwnerID: owner}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&a).Error; err != nil {
		return a, err
	}
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("owner_id=?", owner).Take(&a).Error
	return a, err
}
func advance(tx *gorm.DB, a *model.Album) error {
	if a.Version >= MaxSafeVersion {
		return ErrVersion
	}
	r := tx.Model(&model.Album{}).Where("owner_id=? AND version=?", a.OwnerID, a.Version).Update("version", a.Version+1)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return ErrVersion
	}
	a.Version++
	return nil
}
func audit(tx *gorm.DB, a model.Album, p model.Photo, kind string, actor uint64, id, action, from, to, reason string) error {
	return tx.Create(&model.Audit{PhotoNo: p.PhotoNo, OwnerID: a.OwnerID, ActorKind: kind, ActorID: actor, RequestID: id, Action: action, FromStatus: from, ToStatus: to, Reason: reason, AlbumVersion: a.Version, CreatedAt: time.Now().UnixMilli()}).Error
}

func (s *AlbumService) photoDTO(p model.Photo, kind string) resp.Photo {
	base := s.Options.PublicBase + "/album/preview"
	if kind == "admin" {
		base = s.Options.AdminBase + "/album/preview"
	}
	return resp.Photo{PublicPhoto: publicPhoto(p, base), AnchorNo: p.AnchorNo, Status: p.Status, Version: p.Version, Sort: p.Sort, CreatedAt: p.CreatedAt, Reason: p.Reason}
}
func publicPhoto(p model.Photo, base string) resp.PublicPhoto {
	query := "?photoNo=" + url.QueryEscape(p.PhotoNo)
	return resp.PublicPhoto{PhotoNo: p.PhotoNo, Width: p.Width, Height: p.Height, URL: base + query + "&variant=full", ThumbnailURL: base + query + "&variant=thumb"}
}
func (s *AlbumService) OwnerAlbum(ctx context.Context, owner uint64) (resp.Album, error) {
	out := resp.Album{List: []resp.Photo{}, MaxPhotos: s.Options.MaxPhotos}
	if err := s.ready(); err != nil {
		return out, err
	}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := ownerAnchor(tx, owner); err != nil {
			return err
		}
		var a model.Album
		err := tx.Where("owner_id=?", owner).Take(&a).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		out.Version = a.Version
		var photos []model.Photo
		if err := tx.Where("owner_id=? AND status<>?", owner, model.PhotoDeleted).Order("sort ASC, id ASC").Find(&photos).Error; err != nil {
			return err
		}
		for _, p := range photos {
			out.List = append(out.List, s.photoDTO(p, "owner"))
		}
		return nil
	})
	return out, err
}

// Upload prepares immutable files before taking any database locks. A crash can leave
// an unreferenced file but can never commit a photo pointing to a partially written image.
func (s *AlbumService) Upload(ctx context.Context, owner uint64, r req.AlbumMutation, reader io.Reader) (resp.AlbumResult, error) {
	var out resp.AlbumResult
	if err := s.ready(); err != nil {
		return out, err
	}
	if !validCommand(r) {
		return out, ErrInvalid
	}
	if _, err := ownerAnchor(s.DB.WithContext(ctx), owner); err != nil {
		return out, err
	}
	if !s.acquire(owner) {
		return out, ErrBusy
	}
	defer s.release(owner)
	data, err := io.ReadAll(io.LimitReader(reader, MaxUploadBytes+1))
	if err != nil {
		return out, ErrInvalid
	}
	if int64(len(data)) > MaxUploadBytes || len(data) == 0 {
		return out, ErrInvalid
	}
	sum := sha256.Sum256(data)
	hash := payloadHash("upload", struct {
		Mutation req.AlbumMutation
		Digest   string
	}{r, hex.EncodeToString(sum[:])})
	if result, found, err := replay(s.DB.WithContext(ctx), "owner", owner, r.RequestID, hash); err != nil || found {
		return result, err
	}
	// Refuse stale/full albums before spending CPU. The transaction repeats both checks.
	var a model.Album
	err = s.DB.WithContext(ctx).Where("owner_id=?", owner).Take(&a).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return out, err
	}
	if a.Version != r.Version {
		return out, ErrVersion
	}
	var n int64
	if err = s.DB.WithContext(ctx).Model(&model.Photo{}).Where("owner_id=? AND status<>?", owner, model.PhotoDeleted).Count(&n).Error; err != nil {
		return out, err
	}
	if n >= int64(s.Options.MaxPhotos) {
		return out, ErrQuota
	}
	if err := s.uploadAllowance(s.DB.WithContext(ctx), owner); err != nil {
		return out, err
	}
	prepared, err := s.prepare(ctx, data)
	if err != nil {
		return out, err
	}
	commitAttempted := false
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		a, err := lockAlbum(tx, owner)
		if err != nil {
			return err
		}
		anchor, err := ownerAnchor(tx, owner)
		if err != nil {
			return err
		}
		if result, found, err := replay(tx, "owner", owner, r.RequestID, hash); err != nil || found {
			out = result
			return err
		}
		if a.Version != r.Version {
			return ErrVersion
		}
		var n int64
		if err := tx.Model(&model.Photo{}).Where("owner_id=? AND status<>?", owner, model.PhotoDeleted).Count(&n).Error; err != nil {
			return err
		}
		if n >= int64(s.Options.MaxPhotos) {
			return ErrQuota
		}
		if err := s.uploadAllowance(tx, owner); err != nil {
			return err
		}
		var maxSort int
		if err := tx.Model(&model.Photo{}).Select("COALESCE(MAX(sort),0)").Where("owner_id=? AND status<>?", owner, model.PhotoDeleted).Scan(&maxSort).Error; err != nil {
			return err
		}
		if maxSort >= math.MaxInt32 {
			return ErrQuota
		}
		p := model.Photo{PhotoNo: "ph_" + prepared.key, OwnerID: owner, AnchorNo: anchor.AnchorNo, StorageKey: prepared.key, Status: model.PhotoPending, Version: 1, Sort: maxSort + 1, Width: prepared.width, Height: prepared.height, Bytes: prepared.bytes, CreatedAt: time.Now().UnixMilli(), UpdatedAt: time.Now().UnixMilli()}
		if err := tx.Create(&p).Error; err != nil {
			return err
		}
		if err := advance(tx, &a); err != nil {
			return err
		}
		if err := audit(tx, a, p, "owner", owner, r.RequestID, "upload", "", p.Status, ""); err != nil {
			return err
		}
		out = resp.AlbumResult{Version: a.Version, PhotoNo: p.PhotoNo, PhotoVersion: p.Version}
		if err := recordCommand(tx, "owner", owner, r.RequestID, hash, out); err != nil {
			return err
		}
		commitAttempted = true
		return nil
	})
	// A callback failure means no COMMIT was ever issued. Only these owned files
	// may be removed immediately; uncertain COMMIT errors must retain them.
	if !commitAttempted {
		s.removePrepared(prepared.key)
	}
	return out, err
}

func (s *AlbumService) uploadAllowance(db *gorm.DB, owner uint64) error {
	var count int64
	if err := db.Model(&model.Photo{}).Where("owner_id=? AND created_at>=?", owner, time.Now().Add(-24*time.Hour).UnixMilli()).Count(&count).Error; err != nil {
		return err
	}
	if count >= int64(s.Options.MaxUploadsPerDay) {
		return ErrUploadLimit
	}
	return nil
}
func (s *AlbumService) acquire(owner uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.owners[owner] {
		return false
	}
	select {
	case s.slots <- struct{}{}:
		s.owners[owner] = true
		return true
	default:
		return false
	}
}
func (s *AlbumService) release(owner uint64) {
	s.mu.Lock()
	delete(s.owners, owner)
	<-s.slots
	s.mu.Unlock()
}

func (s *AlbumService) Reorder(ctx context.Context, owner uint64, r req.ReorderPhotos) (resp.AlbumResult, error) {
	if !validCommand(r.AlbumMutation) || len(r.PhotoNos) > 30 {
		return resp.AlbumResult{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, no := range r.PhotoNos {
		if !validPhotoNo(no) || seen[no] {
			return resp.AlbumResult{}, ErrInvalid
		}
		seen[no] = true
	}
	return s.ownerMutation(ctx, owner, r.AlbumMutation, "reorder", r, func(tx *gorm.DB, a model.Album) (resp.AlbumResult, error) {
		var photos []model.Photo
		if err := tx.Where("owner_id=? AND status<>?", owner, model.PhotoDeleted).Find(&photos).Error; err != nil {
			return resp.AlbumResult{}, err
		}
		if len(photos) != len(r.PhotoNos) {
			return resp.AlbumResult{}, ErrVersion
		}
		for _, p := range photos {
			if !seen[p.PhotoNo] {
				return resp.AlbumResult{}, ErrVersion
			}
		}
		for i, no := range r.PhotoNos {
			if err := tx.Model(&model.Photo{}).Where("owner_id=? AND photo_no=?", owner, no).Updates(map[string]any{"sort": i + 1, "updated_at": time.Now().UnixMilli()}).Error; err != nil {
				return resp.AlbumResult{}, err
			}
		}
		if err := audit(tx, a, model.Photo{}, "owner", owner, r.RequestID, "reorder", "", "", ""); err != nil {
			return resp.AlbumResult{}, err
		}
		return resp.AlbumResult{Version: a.Version}, nil
	})
}
func (s *AlbumService) Delete(ctx context.Context, owner uint64, r req.DeletePhoto) (resp.AlbumResult, error) {
	if !validCommand(r.AlbumMutation) || !validPhotoNo(r.PhotoNo) {
		return resp.AlbumResult{}, ErrInvalid
	}
	return s.ownerMutation(ctx, owner, r.AlbumMutation, "delete", r, func(tx *gorm.DB, a model.Album) (resp.AlbumResult, error) {
		var p model.Photo
		err := tx.Where("owner_id=? AND photo_no=? AND status<>?", owner, r.PhotoNo, model.PhotoDeleted).Take(&p).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return resp.AlbumResult{}, ErrNotFound
		}
		if err != nil {
			return resp.AlbumResult{}, err
		}
		if p.Version >= MaxSafeVersion {
			return resp.AlbumResult{}, ErrVersion
		}
		from, version := p.Status, p.Version+1
		if err := tx.Model(&p).Updates(map[string]any{"status": model.PhotoDeleted, "version": version, "updated_at": time.Now().UnixMilli()}).Error; err != nil {
			return resp.AlbumResult{}, err
		}
		if err := audit(tx, a, p, "owner", owner, r.RequestID, "delete", from, model.PhotoDeleted, ""); err != nil {
			return resp.AlbumResult{}, err
		}
		return resp.AlbumResult{Version: a.Version, PhotoNo: p.PhotoNo, PhotoVersion: version}, nil
	})
}
func (s *AlbumService) ownerMutation(ctx context.Context, owner uint64, r req.AlbumMutation, action string, payload any, change func(*gorm.DB, model.Album) (resp.AlbumResult, error)) (resp.AlbumResult, error) {
	var out resp.AlbumResult
	if err := s.ready(); err != nil {
		return out, err
	}
	hash := payloadHash(action, payload)
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		a, err := lockAlbum(tx, owner)
		if err != nil {
			return err
		}
		if _, err := ownerAnchor(tx, owner); err != nil {
			return err
		}
		if result, found, err := replay(tx, "owner", owner, r.RequestID, hash); err != nil || found {
			out = result
			return err
		}
		if a.Version != r.Version {
			return ErrVersion
		}
		if err := advance(tx, &a); err != nil {
			return err
		}
		out, err = change(tx, a)
		if err != nil {
			return err
		}
		return recordCommand(tx, "owner", owner, r.RequestID, hash, out)
	})
	return out, err
}

func (s *AlbumService) Review(ctx context.Context, actor uint64, r req.ReviewPhoto) (resp.AlbumResult, error) {
	var out resp.AlbumResult
	if err := s.ready(); err != nil {
		return out, err
	}
	r.Reason = strings.TrimSpace(r.Reason)
	if !validCommand(r.AlbumMutation) || !validPhotoNo(r.PhotoNo) || !utf8.ValidString(r.Reason) || len(r.Reason) > 800 || utf8.RuneCountInString(r.Reason) > 200 {
		return out, ErrInvalid
	}
	for _, v := range r.Reason {
		if unicode.IsControl(v) {
			return out, ErrInvalid
		}
	}
	if r.Decision != "approve" && r.Decision != "reject" && r.Decision != "takedown" {
		return out, ErrInvalid
	}
	if r.Decision != "approve" && r.Reason == "" {
		return out, ErrInvalid
	}
	hash := payloadHash("review", r)
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Serialize one administrator's commands before locking different albums;
		// this also makes requestId reuse across owners deterministic under MySQL RR.
		if err := albumAdmin(tx.Clauses(clause.Locking{Strength: "UPDATE"}), actor); err != nil {
			return err
		}
		var p model.Photo
		if err := tx.Where("photo_no=?", r.PhotoNo).Take(&p).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		a, err := lockAlbum(tx, p.OwnerID)
		if err != nil {
			return err
		}
		if result, found, err := replay(tx, "admin", actor, r.RequestID, hash); err != nil || found {
			out = result
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("photo_no=?", r.PhotoNo).Take(&p).Error; err != nil {
			return err
		}
		if p.Version != r.Version || p.Version >= MaxSafeVersion {
			return ErrVersion
		}
		from := p.Status
		target := model.PhotoApproved
		switch r.Decision {
		case "approve", "reject":
			if p.Status != model.PhotoPending {
				return ErrVersion
			}
			if r.Decision == "reject" {
				target = model.PhotoRejected
			}
		case "takedown":
			if p.Status != model.PhotoApproved {
				return ErrVersion
			}
			target = model.PhotoRemoved
		}
		if target == model.PhotoApproved {
			if _, err := ownerAnchor(tx, p.OwnerID); err != nil {
				return err
			}
		}
		if err := advance(tx, &a); err != nil {
			return err
		}
		if err := tx.Model(&p).Updates(map[string]any{"status": target, "version": p.Version + 1, "reason": r.Reason, "updated_at": time.Now().UnixMilli()}).Error; err != nil {
			return err
		}
		if err := audit(tx, a, p, "admin", actor, r.RequestID, r.Decision, from, target, r.Reason); err != nil {
			return err
		}
		out = resp.AlbumResult{Version: a.Version, PhotoNo: p.PhotoNo, PhotoVersion: r.Version + 1}
		return recordCommand(tx, "admin", actor, r.RequestID, hash, out)
	})
	return out, err
}

func (s *AlbumService) AdminList(ctx context.Context, actor uint64, q req.AlbumSearch) (resp.AlbumPage, error) {
	out := resp.AlbumPage{List: []resp.Photo{}}
	if err := s.ready(); err != nil {
		return out, err
	}
	db := s.DB.WithContext(ctx)
	if err := albumAdmin(db, actor); err != nil {
		return out, err
	}
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}
	if q.Page < 1 || q.Page > 1000 || q.PageSize < 1 || q.PageSize > 100 {
		return out, ErrInvalid
	}
	db = db.Model(&model.Photo{})
	if q.Status != "" {
		switch q.Status {
		case model.PhotoPending, model.PhotoApproved, model.PhotoRejected, model.PhotoRemoved, model.PhotoDeleted:
		default:
			return out, ErrInvalid
		}
		db = db.Where("status=?", q.Status)
	}
	if q.AnchorNo != "" {
		if len(q.AnchorNo) > 32 || !opaque.MatchString(q.AnchorNo) {
			return out, ErrInvalid
		}
		db = db.Where("anchor_no=?", q.AnchorNo)
	}
	if err := db.Count(&out.Total).Error; err != nil {
		return out, err
	}
	var photos []model.Photo
	if err := db.Order("created_at DESC, id DESC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Find(&photos).Error; err != nil {
		return out, err
	}
	for _, p := range photos {
		out.List = append(out.List, s.photoDTO(p, "admin"))
	}
	out.Page = q.Page
	out.PageSize = q.PageSize
	return out, nil
}

func (s *AlbumService) ApprovedPhotos(ctx context.Context, viewer uint64, anchorNo string) ([]resp.PublicPhoto, error) {
	return s.ApprovedPhotosFromDB(ctx, s.DB, viewer, anchorNo)
}
func (s *AlbumService) ApprovedPhotosFromDB(ctx context.Context, db *gorm.DB, viewer uint64, anchorNo string) ([]resp.PublicPhoto, error) {
	out := []resp.PublicPhoto{}
	if err := s.ready(); err != nil {
		return out, err
	}
	a, err := VisibleAnchor(ctx, db, viewer, anchorNo)
	if err != nil {
		return out, err
	}
	var photos []model.Photo
	if err := db.WithContext(ctx).Where("owner_id=? AND status=?", a.UserId, model.PhotoApproved).Order("sort ASC, id ASC").Limit(30).Find(&photos).Error; err != nil {
		return out, err
	}
	for _, p := range photos {
		out = append(out, publicPhoto(p, s.Options.PublicBase+"/photo"))
	}
	return out, nil
}
