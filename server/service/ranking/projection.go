package ranking

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math/big"
	"sort"
	"strings"
	"tb_live_module/internal/modulekit"
	gift "tb_live_module/model/gift"
	model "tb_live_module/model/ranking"
	"time"
)

type dimensions struct{ User, Anchor, Room, Session, Category, Gift string }

func PeriodKey(period string, ms int64, zone string) (string, error) {
	loc, e := time.LoadLocation(zone)
	if e != nil {
		return "", e
	}
	t := time.UnixMilli(ms).In(loc)
	switch period {
	case "all":
		return "all", nil
	case "day":
		return t.Format("2006-01-02"), nil
	case "month":
		return t.Format("2006-01"), nil
	case "week":
		t = t.AddDate(0, 0, -(int(t.Weekday())+6)%7)
		return t.Format("2006-01-02"), nil
	}
	return "", ErrInvalid
}
func addDecimal(value string, delta int64) (string, error) {
	n, ok := new(big.Int).SetString(value, 10)
	if !ok || n.Sign() < 0 || n.String() != value {
		return "", ErrInvariant
	}
	n.Add(n, big.NewInt(delta))
	if n.Sign() < 0 || len(n.String()) > 65 {
		return "", ErrInvariant
	}
	return n.String(), nil
}
func sortKey(value string) string {
	if len(value) > 65 {
		return ""
	}
	return strings.Repeat("0", 65-len(value)) + value
}
func keys(d dimensions, at int64, zone string) ([]model.ScoreBucket, error) {
	bases := []model.ScoreBucket{}
	for _, scope := range []struct{ kind, no string }{{"anchor", d.Anchor}, {"room", d.Room}, {"session", d.Session}} {
		for _, g := range []string{"", d.Gift} {
			bases = append(bases, model.ScoreBucket{RankType: "gift", ScopeType: scope.kind, ScopeNo: scope.no, SubjectType: "user", SubjectNo: d.User, GiftNo: g})
		}
	}
	bases = append(bases, model.ScoreBucket{RankType: "wealth", ScopeType: "platform", SubjectType: "user", SubjectNo: d.User}, model.ScoreBucket{RankType: "charm", ScopeType: "platform", SubjectType: "anchor", SubjectNo: d.Anchor}, model.ScoreBucket{RankType: "charm", ScopeType: "category", ScopeNo: d.Category, SubjectType: "anchor", SubjectNo: d.Anchor})
	result := make([]model.ScoreBucket, 0, len(bases)*4)
	for _, b := range bases {
		if b.SubjectNo == "" || b.ScopeType != "platform" && b.ScopeNo == "" {
			return nil, ErrInvariant
		}
		for _, period := range []string{"day", "week", "month", "all"} {
			key, e := PeriodKey(period, at, zone)
			if e != nil {
				return nil, e
			}
			b.PeriodType = period
			b.PeriodKey = key
			result = append(result, b)
		}
	}
	return result, nil
}
func bucketIdentity(b model.ScoreBucket) string {
	raw, _ := json.Marshal([]any{b.Generation, b.RankType, b.PeriodType, b.PeriodKey, b.ScopeType, b.ScopeNo, b.GiftNo, b.SubjectType, b.SubjectNo})
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

// The checkpoint lock serializes writers. Batch reads and upserts avoid one SQL round-trip per delta.
func applyBatch(tx *gorm.DB, events []model.AppliedEvent, generation uint64, zone string, now int64) error {
	type delta struct {
		key            string
		coin, quantity int64
	}
	changes := []delta{}
	buckets := map[string]model.ScoreBucket{}
	ids := []string{}
	for _, event := range events {
		var d dimensions
		if json.Unmarshal([]byte(event.Dimensions), &d) != nil {
			return ErrInvariant
		}
		keys, e := keys(d, event.OriginalEventAt, zone)
		if e != nil {
			return e
		}
		for _, b := range keys {
			b.Generation = generation
			b.BucketKey = bucketIdentity(b)
			if _, exists := buckets[b.BucketKey]; !exists {
				b.ScoreCoin = "0"
				b.Quantity = "0"
				buckets[b.BucketKey] = b
				ids = append(ids, b.BucketKey)
			}
			changes = append(changes, delta{b.BucketKey, event.DeltaCoin, event.DeltaQuantity})
		}
	}
	for start := 0; start < len(ids); start += 200 {
		var rows []model.ScoreBucket
		if e := tx.Where("bucket_key IN ?", ids[start:min(start+200, len(ids))]).Find(&rows).Error; e != nil {
			return e
		}
		for _, b := range rows {
			if bucketIdentity(b) != b.BucketKey || b.Generation != generation {
				return ErrInvariant
			}
			buckets[b.BucketKey] = b
		}
	}
	for _, change := range changes {
		b := buckets[change.key]
		var e error
		b.ScoreCoin, e = addDecimal(b.ScoreCoin, change.coin)
		if e != nil {
			return e
		}
		b.Quantity, e = addDecimal(b.Quantity, change.quantity)
		if e != nil {
			return e
		}
		b.ScoreSort = sortKey(b.ScoreCoin)
		b.Version++
		b.UpdatedAt = now
		buckets[change.key] = b
	}
	sorted := make([]model.ScoreBucket, 0, len(ids))
	sort.Strings(ids)
	for _, key := range ids {
		sorted = append(sorted, buckets[key])
	}
	if len(sorted) > 0 {
		if e := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "bucket_key"}}, DoUpdates: clause.AssignmentColumns([]string{"score_coin", "quantity", "score_sort", "version", "updated_at"})}).CreateInBatches(&sorted, 100).Error; e != nil {
			return e
		}
	}
	receipts := make([]model.Receipt, 0, len(events))
	for _, event := range events {
		receipts = append(receipts, model.Receipt{Generation: generation, SourceID: event.SourceID})
	}
	if len(receipts) == 0 {
		return nil
	}
	return tx.CreateInBatches(&receipts, 100).Error
}
func checkpoint(tx *gorm.DB, zone string) (model.Checkpoint, error) {
	var cp model.Checkpoint
	e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("consumer_name='gift-revenue'").Take(&cp).Error
	if e == nil && cp.Timezone != zone {
		return cp, ErrInvariant
	}
	return cp, e
}

// NOT EXISTS receipts, rather than source_id > cursor, also consumes late commits
// whose auto-increment IDs are below an already observed source ID.
func (s *Service) Consume(ctx context.Context, batch int) (int, error) {
	if batch < 1 || batch > 200 {
		return 0, ErrInvalid
	}
	count := 0
	e := s.transaction(ctx, func(tx *gorm.DB) error {
		cp, e := checkpoint(tx, s.zone())
		if e != nil {
			return e
		}
		now := s.now()
		if cp.LeaseUntil > now && cp.LastError != "" {
			return modulekit.Unavailable
		}
		cp.LeaseToken = modulekit.No("rl_")
		cp.LeaseUntil = now + 30000
		var jobs []model.RebuildJob
		if e = tx.Where("status='running'").Limit(2).Find(&jobs).Error; e != nil {
			return e
		}
		if len(jobs) > 1 {
			return ErrInvariant
		}
		gen := cp.Generation
		if len(jobs) == 1 {
			gen = jobs[0].Generation
		}
		var sources []gift.Revenue
		if e = tx.Table("gift_revenue_ledger AS r").Select("r.*").Where("NOT EXISTS (SELECT 1 FROM ranking_generation_receipt p WHERE p.generation=? AND p.source_id=r.id)", gen).Order("r.id").Limit(batch).Find(&sources).Error; e != nil {
			return e
		}
		orders := map[string]gift.Order{}
		numbers := []string{}
		for _, r := range sources {
			numbers = append(numbers, r.GiftOrderNo)
		}
		if len(numbers) > 0 {
			var rows []gift.Order
			if e = tx.Where("order_no IN ?", numbers).Find(&rows).Error; e != nil {
				return e
			}
			for _, o := range rows {
				orders[o.OrderNo] = o
			}
		}
		events := []model.AppliedEvent{}
		missing := []model.AppliedEvent{}
		priorByID := map[uint64]model.AppliedEvent{}
		sourceIDs := []uint64{}
		for _, r := range sources {
			sourceIDs = append(sourceIDs, r.ID)
		}
		if len(sourceIDs) > 0 {
			var prior []model.AppliedEvent
			if e = tx.Where("source_id IN ?", sourceIDs).Find(&prior).Error; e != nil {
				return e
			}
			for _, p := range prior {
				priorByID[p.SourceID] = p
			}
		}
		for _, r := range sources {
			o, ok := orders[r.GiftOrderNo]
			if !ok || o.UnitPrice <= 0 || r.Quantity == 0 || r.GrossCoinAmount == 0 || (r.EntryType == "gift") != (r.GrossCoinAmount > 0) || r.GrossCoinAmount/o.UnitPrice != r.Quantity || r.GrossCoinAmount%o.UnitPrice != 0 {
				return ErrInvariant
			}
			dims, _ := json.Marshal(dimensions{o.SenderAccountNo, o.AnchorNo, o.RoomNo, o.SessionNo, o.CategoryNo, o.GiftNo})
			event := model.AppliedEvent{EventNo: r.RevenueNo, SourceType: r.EntryType, SourceNo: r.BusinessNo, SourceID: r.ID, DeltaCoin: r.GrossCoinAmount, DeltaQuantity: r.Quantity, OriginalEventAt: o.CreatedAt, Dimensions: string(dims), AppliedAt: now}
			if prior, exists := priorByID[r.ID]; exists {
				if prior.SourceID != event.SourceID || prior.DeltaCoin != event.DeltaCoin || prior.DeltaQuantity != event.DeltaQuantity || prior.Dimensions != event.Dimensions || prior.OriginalEventAt != event.OriginalEventAt {
					return ErrInvariant
				}
			} else {
				missing = append(missing, event)
			}
			events = append(events, event)
			cp.LastSourceID = max(cp.LastSourceID, r.ID)
			count++
		}
		if len(missing) > 0 {
			if e = tx.CreateInBatches(&missing, 100).Error; e != nil {
				return e
			}
		}
		if e = applyBatch(tx, events, gen, s.zone(), now); e != nil {
			return e
		}
		if len(jobs) == 1 {
			job := jobs[0]
			job.ProcessedCount += uint64(count)
			job.Checkpoint = cp.LastSourceID
			if count == 0 {
				job.Status = "succeeded"
				job.CompletedAt = now
				cp.Generation = gen
			}
			if e = tx.Save(&job).Error; e != nil {
				return e
			}
		}
		cp.LeaseToken = ""
		cp.LeaseUntil = 0
		cp.LastError = ""
		cp.UpdatedAt = now
		if count == 0 {
			cp.DataAsOf = now
		}
		return tx.Save(&cp).Error
	})
	if e != nil && e != modulekit.Unavailable {
		finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = s.db(finish).Transaction(func(tx *gorm.DB) error {
			if e := tx.Model(&model.Checkpoint{}).Where("consumer_name='gift-revenue'").Updates(map[string]any{"last_error": "榜单投影失败，请核验来源与任务", "lease_until": s.now() + 30000, "updated_at": s.now()}).Error; e != nil {
				return e
			}
			return tx.Model(&model.RebuildJob{}).Where("status='running'").Updates(map[string]any{"error_count": gorm.Expr("error_count+1"), "last_error": "投影处理失败，进度保留等待重试"}).Error
		})
	}
	return count, e
}
