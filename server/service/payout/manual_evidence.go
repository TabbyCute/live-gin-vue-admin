package payout

import (
	"context"
	"encoding/base64"
	"gorm.io/gorm"
	model "tb_live_module/model/payout"
	req "tb_live_module/model/payout/request"
	res "tb_live_module/model/payout/response"
	"tb_live_module/model/system"
	"time"
)

func (s *Service) UploadEvidence(ctx context.Context, a Actor, trace string, r req.UploadEvidence) (res.ActionResult, error) {
	var empty res.ActionResult
	if !attemptNoRE.MatchString(r.AttemptNo) || len(r.Content) > base64.StdEncoding.EncodedLen(MaxEvidenceBytes) || s.EvidenceStore == nil {
		return empty, ErrEvidence
	}
	b, e := base64.StdEncoding.Strict().DecodeString(r.Content)
	if e != nil {
		return empty, ErrEvidence
	}
	defer clear(b)
	hash, e := validateEvidence(r.MIME, b)
	if e != nil {
		return empty, e
	}
	var written []model.Evidence
	out, e := s.manualCommand(ctx, a, "evidence/upload", r.RequestID, r, func(tx *gorm.DB) (res.ActionResult, error) {
		o, at, e := s.lockAttempt(tx, r.AttemptNo)
		if e != nil {
			return empty, e
		}
		if o.Status == "processing" && at.Status == "processing" {
			if at.OperatorID != a.ID {
				return empty, ErrOrderForbidden
			}
		} else if o.Status == "unknown" && at.Status == "unknown" {
			if e = s.authorizePayout(tx, a, "attempt/resolve-unknown", "POST"); e != nil {
				return empty, e
			}
			if at.OperatorID == a.ID && !system.IsSuperAdmin(a.ID) {
				return empty, ErrSelfReview
			}
		} else {
			return empty, ErrOrderState
		}
		var count int64
		if e = tx.Model(&model.Evidence{}).Where("attempt_no=?", at.AttemptNo).Count(&count).Error; e != nil {
			return empty, e
		}
		if count >= 10 {
			return empty, ErrEvidence
		}
		proof := model.Evidence{EvidenceNo: number("pev_"), PayoutNo: o.PayoutNo, AttemptNo: at.AttemptNo, ObjectReference: number("peo_"), SHA256: hash, SourceSHA256: hash, MIME: r.MIME, Size: int64(len(b)), EncryptionKeyID: s.Crypto.active, CreatedBy: a.ID, CreatedAt: s.now()}
		sealed, e := s.Crypto.sealBytes(proof.EncryptionKeyID, evidenceAAD(proof), b)
		if e != nil {
			return empty, e
		}
		if e = s.EvidenceStore.Put(ctx, proof.ObjectReference, sealed); e != nil {
			return empty, e
		}
		written = append(written, proof)
		if e = tx.Create(&proof).Error; e != nil {
			return empty, e
		}
		if _, e = auditManual(tx, a, trace, o.PayoutNo, proof.EvidenceNo, "evidence.upload", map[string]any{"attemptNo": at.AttemptNo, "evidenceNo": proof.EvidenceNo, "sha256": proof.SHA256, "mime": proof.MIME, "size": proof.Size}, proof.CreatedAt); e != nil {
			return empty, e
		}
		return res.ActionResult{PayoutNo: o.PayoutNo, AttemptNo: at.AttemptNo, EvidenceNo: proof.EvidenceNo, EvidenceHash: proof.SHA256, Status: o.Status}, nil
	})
	// On an ambiguous commit, never delete a possibly referenced object. A
	// later authenticated maintenance pass can inspect unreferenced objects.
	for _, proof := range written {
		if proof.EvidenceNo == out.EvidenceNo && e == nil {
			continue
		}
		checkCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		var count int64
		check := s.db(checkCtx).Model(&model.Evidence{}).Where("object_reference=?", proof.ObjectReference).Count(&count).Error
		cancel()
		if check == nil && count == 0 {
			_ = s.EvidenceStore.Remove(proof.ObjectReference)
		}
	}
	return out, e
}

func (s *Service) readEvidenceBytes(ctx context.Context, proof model.Evidence) ([]byte, error) {
	if s.EvidenceStore == nil || proof.Size <= 0 || proof.Size > MaxEvidenceBytes {
		return nil, ErrEvidence
	}
	sealed, e := s.EvidenceStore.Read(ctx, proof.ObjectReference)
	if e != nil {
		return nil, e
	}
	b, e := s.Crypto.openBytes(proof.EncryptionKeyID, evidenceAAD(proof), sealed)
	if e != nil {
		return nil, e
	}
	if int64(len(b)) != proof.Size || snapshotHash(string(b)) != proof.SHA256 {
		clear(b)
		return nil, ErrEvidence
	}
	return b, nil
}
func (s *Service) ReadEvidence(ctx context.Context, a Actor, trace string, r req.ReadEvidence) ([]byte, string, error) {
	if e := s.authorizePayout(s.db(ctx), a, "order/detail", "GET"); e != nil {
		return nil, "", e
	}
	reason, e := safeNote(r.Reason)
	if e != nil || !evidenceNoRE.MatchString(r.EvidenceNo) {
		return nil, "", ErrOrderInvalid
	}
	_, e = s.manualCommand(ctx, a, "evidence/read", r.RequestID, r, func(tx *gorm.DB) (res.ActionResult, error) {
		var proof model.Evidence
		if e := tx.Where("evidence_no=?", r.EvidenceNo).Take(&proof).Error; e != nil {
			return res.ActionResult{}, ErrEvidence
		}
		o, at, e := s.lockAttempt(tx, proof.AttemptNo)
		if e != nil {
			return res.ActionResult{}, e
		}
		if at.OperatorID != a.ID {
			if e = s.authorizePayout(tx, a, "order/detail", "GET"); e != nil {
				return res.ActionResult{}, e
			}
		}
		_, e = auditManual(tx, a, trace, o.PayoutNo, proof.EvidenceNo, "evidence.read", map[string]any{"evidenceNo": proof.EvidenceNo, "reason": reason, "accessAuthorized": true}, s.now())
		return res.ActionResult{PayoutNo: o.PayoutNo, AttemptNo: at.AttemptNo, EvidenceNo: proof.EvidenceNo}, e
	})
	if e != nil {
		return nil, "", e
	}
	var proof model.Evidence
	if e = s.db(ctx).Where("evidence_no=?", r.EvidenceNo).Take(&proof).Error; e != nil {
		return nil, "", ErrEvidence
	}
	b, e := s.readEvidenceBytes(ctx, proof)
	return b, proof.MIME, e
}
