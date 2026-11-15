package bankreconcile

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	model "tb_live_module/model/bankreconcile"
	req "tb_live_module/model/bankreconcile/request"
	pm "tb_live_module/model/payout"
	"tb_live_module/model/system"
	payout "tb_live_module/service/payout"
	"time"
	"unicode"
	"unicode/utf8"
)

const MaxCSVBytes = 512 << 10
const MaxRows = 1000

var keyRE = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)
var noRE = regexp.MustCompile(`^(bb|br|bc)_[a-f0-9]{32}$`)
var companyRE = regexp.MustCompile(`^pca_[a-f0-9]{32}$`)
var currencyRE = regexp.MustCompile(`^[A-Z]{3,8}$`)
var redactRE = regexp.MustCompile(`[0-9]{6,}|[A-Za-z0-9_.+%-]+@[A-Za-z0-9.-]+|https?://\S+`)
var (
	ErrInvalid    = &payout.Error{Code: 49301, Message: "银行核对参数或CSV格式无效"}
	ErrConflict   = &payout.Error{Code: 49302, Message: "请求键或银行流水已绑定不同内容，请调查原记录"}
	ErrLimit      = &payout.Error{Code: 49303, Message: "账单超过大小、行数或核对范围限制，请缩小账期"}
	ErrState      = &payout.Error{Code: 49304, Message: "核对记录不存在或当前状态不允许处理"}
	ErrSelfReview = &payout.Error{Code: 49305, Message: "导入人、核对操作人和出款操作人不能复核自身记录"}
)

func number(prefix string) string { return prefix + strings.ReplaceAll(uuid.NewString(), "-", "") }
func now() int64                  { return time.Now().UnixMilli() }
func safeText(v string, max int) bool {
	return utf8.ValidString(v) && len(v) > 0 && len(v) <= max && strings.TrimSpace(v) == v && strings.IndexFunc(v, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) < 0
}

type Result struct {
	No string `json:"no"`
}

func (s *Service) command(ctx context.Context, a payout.Actor, trace, action, key string, input any, fn func(*gorm.DB) (string, error)) (Result, error) {
	if !keyRE.MatchString(key) {
		return Result{}, ErrInvalid
	}
	digest, err := s.payout().BankReconciliationDigest("request", []any{a.ID, action, input})
	if err != nil {
		return Result{}, err
	}
	for retry := 0; retry < 4; retry++ {
		var out Result
		isolation := sql.LevelReadCommitted
		if action == "run" {
			isolation = sql.LevelRepeatableRead
		}
		err = s.db(ctx).Transaction(func(tx *gorm.DB) error {
			if e := s.payout().BankReconciliationAuthorize(tx, a, action, "POST"); e != nil {
				return e
			}
			// Unique receipt insertion rolls back any concurrent request-key loser.
			receipt := model.Receipt{RequestID: key, ActorID: a.ID, Action: action, Digest: digest, CreatedAt: now()}
			var old model.Receipt
			e := tx.Where("request_id=?", key).Take(&old).Error
			if e == nil {
				if old.ActorID != a.ID || old.Action != action || old.Digest != digest {
					return ErrConflict
				}
				out.No = old.ResultNo
				return nil
			}
			if !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
			// No placeholder is committed: all evidence, audit and receipt share the tx.
			out.No, e = fn(tx)
			if e != nil {
				return e
			}
			receipt.ResultNo = out.No
			if e = tx.Create(&receipt).Error; e != nil {
				return e
			}
			if len(trace) > 64 {
				trace = trace[:64]
			}
			return tx.Create(&model.Audit{EventNo: number("ba_"), Action: action, ObjectNo: out.No, ActorID: a.ID, TraceID: trace, CreatedAt: now()}).Error
		}, &sql.TxOptions{Isolation: isolation})
		// A contender may have waited on a check/account lock after its first
		// receipt read. Resolve a concurrently committed identical command before
		// reporting a now-stale state error. Recheck live authorization on replay.
		if err != nil && ctx.Err() == nil {
			var replay Result
			replayErr := s.db(ctx).Transaction(func(tx *gorm.DB) error {
				if e := s.payout().BankReconciliationAuthorize(tx, a, action, "POST"); e != nil {
					return e
				}
				var receipt model.Receipt
				if e := tx.Where("request_id=?", key).Take(&receipt).Error; e != nil {
					return e
				}
				if receipt.ActorID != a.ID || receipt.Action != action || receipt.Digest != digest {
					return ErrConflict
				}
				replay.No = receipt.ResultNo
				return nil
			}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
			if replayErr == nil {
				return replay, nil
			}
			if errors.Is(replayErr, ErrConflict) || errors.Is(replayErr, payout.ErrOrderForbidden) {
				return Result{}, replayErr
			}
		}
		var me *driver.MySQLError
		if err == nil {
			return out, nil
		}
		if !errors.As(err, &me) || (me.Number != 1062 && me.Number != 1213 && me.Number != 1205) {
			return Result{}, err
		}
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
	}
	return Result{}, ErrConflict
}

// Import accepts a normalized CSV assertion; it never saves raw input or payment proof.
func (s *Service) Import(ctx context.Context, a payout.Actor, trace string, r req.Import) (Result, error) {
	if !companyRE.MatchString(r.ChannelAccountNo) || r.PeriodFrom <= 0 || r.PeriodTo <= r.PeriodFrom || r.PeriodTo-r.PeriodFrom > 93*86400000 || r.PeriodTo > now()+86400000 {
		return Result{}, ErrInvalid
	}
	if len(r.CSV) > MaxCSVBytes {
		return Result{}, ErrLimit
	}
	if !utf8.ValidString(r.CSV) || strings.ContainsRune(r.CSV, '\x00') {
		return Result{}, ErrInvalid
	}
	return s.command(ctx, a, trace, "import", r.RequestID, r, func(tx *gorm.DB) (string, error) {
		var account pm.ChannelAccount
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("channel_account_no=?", r.ChannelAccountNo).Take(&account).Error; e != nil {
			return "", ErrState
		}
		reader := csv.NewReader(strings.NewReader(r.CSV))
		reader.FieldsPerRecord = 8
		header, e := reader.Read()
		if e != nil || strings.Join(header, ",") != "bank_reference,company_account,recipient_account,currency,amount_minor,status,booked_at,direction" {
			return "", ErrInvalid
		}
		rows := make([]model.Line, 0)
		seen := map[string]bool{}
		for {
			fields, e := reader.Read()
			if e == io.EOF {
				break
			}
			if e != nil {
				return "", ErrInvalid
			}
			if len(rows) >= MaxRows {
				return "", ErrLimit
			}
			amount, e := strconv.ParseInt(fields[4], 10, 64)
			if e != nil || amount <= 0 || strconv.FormatInt(amount, 10) != fields[4] {
				return "", ErrInvalid
			}
			booked, e := time.Parse(time.RFC3339, fields[6])
			if e != nil || booked.UnixMilli() < r.PeriodFrom || booked.UnixMilli() >= r.PeriodTo {
				return "", ErrInvalid
			}
			if !currencyRE.MatchString(fields[3]) || fields[3] == "COIN" || fields[7] != "debit" {
				return "", ErrInvalid
			}
			switch fields[5] {
			case "settled", "pending", "failed", "reversed":
			default:
				return "", ErrInvalid
			}
			fp, mask, e := s.payout().BankStatementIdentity(account, fields[1], fields[0])
			if e != nil {
				return "", ErrInvalid
			}
			recipient, e := s.payout().BankRecipientFingerprint(fields[2])
			if e != nil {
				return "", ErrInvalid
			}
			if seen[fp] {
				return "", ErrConflict
			}
			seen[fp] = true
			line := model.Line{LineNo: number("bl_"), ChannelAccountNo: r.ChannelAccountNo, ReferenceFingerprint: fp, ReferenceMasked: mask, RecipientFingerprint: recipient, Currency: fields[3], Amount: amount, BankStatus: fields[5], BookedAt: booked.UnixMilli()}
			line.ContentHash, e = s.payout().BankReconciliationDigest("line", []any{r.ChannelAccountNo, fp, recipient, line.Currency, line.Amount, line.BankStatus, line.BookedAt})
			if e != nil {
				return "", e
			}
			rows = append(rows, line)
		}
		if len(rows) == 0 {
			return "", ErrInvalid
		}
		// Fingerprinted normalized records, not a stored raw file hash containing secret data.
		hashes := make([]string, len(rows))
		for i := range rows {
			hashes[i] = rows[i].ContentHash
		}
		sort.Strings(hashes)
		source, e := s.payout().BankReconciliationDigest("batch", []any{r.ChannelAccountNo, r.PeriodFrom, r.PeriodTo, r.DeclaredComplete, hashes})
		if e != nil {
			return "", e
		}
		var old model.Batch
		e = tx.Where("channel_account_no=? AND source_hash=?", r.ChannelAccountNo, source).Take(&old).Error
		if e == nil {
			return old.BatchNo, nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return "", e
		}
		batch := model.Batch{BatchNo: number("bb_"), ChannelAccountNo: r.ChannelAccountNo, CompanyMasked: account.MaskedAccount, Currency: account.Currency, SourceHash: source, PeriodFrom: r.PeriodFrom, PeriodTo: r.PeriodTo, DeclaredComplete: r.DeclaredComplete, RowCount: len(rows), CreatedBy: a.ID, CreatedAt: now()}
		if e = tx.Create(&batch).Error; e != nil {
			return "", e
		}
		references := make([]string, 0, len(rows))
		for _, line := range rows {
			references = append(references, line.ReferenceFingerprint)
		}
		var existingRows []model.Line
		if e = tx.Where("channel_account_no=? AND reference_fingerprint IN ?", r.ChannelAccountNo, references).Limit(MaxRows + 1).Find(&existingRows).Error; e != nil {
			return "", e
		}
		existingByReference := map[string]model.Line{}
		for _, line := range existingRows {
			existingByReference[line.ReferenceFingerprint] = line
		}
		newRows := make([]model.Line, 0, len(rows))
		links := make([]model.BatchLine, 0, len(rows))
		for _, line := range rows {
			if existing, found := existingByReference[line.ReferenceFingerprint]; found {
				if existing.ContentHash != line.ContentHash {
					return "", ErrConflict
				}
				line = existing
			} else {
				newRows = append(newRows, line)
			}
			links = append(links, model.BatchLine{BatchNo: batch.BatchNo, LineNo: line.LineNo})
		}
		if len(newRows) > 0 {
			if e = tx.CreateInBatches(&newRows, 100).Error; e != nil {
				return "", e
			}
		}
		if e = tx.CreateInBatches(&links, 100).Error; e != nil {
			return "", e
		}
		return batch.BatchNo, nil
	})
}

func (s *Service) Run(ctx context.Context, a payout.Actor, trace string, r req.Run) (Result, error) {
	if !noRE.MatchString(r.BatchNo) || !strings.HasPrefix(r.BatchNo, "bb_") {
		return Result{}, ErrInvalid
	}
	return s.command(ctx, a, trace, "run", r.RequestID, r, func(tx *gorm.DB) (string, error) {
		var batch model.Batch
		if e := tx.Where("batch_no=?", r.BatchNo).Take(&batch).Error; e != nil {
			return "", ErrState
		}
		var lines []model.Line
		if e := tx.Table("payout_bank_line l").Select("l.*").Joins("JOIN payout_bank_batch_line b ON b.line_no=l.line_no").Where("b.batch_no=?", batch.BatchNo).Limit(MaxRows + 1).Find(&lines).Error; e != nil {
			return "", e
		}
		if len(lines) != batch.RowCount {
			return "", ErrState
		}
		run := model.Run{RunNo: number("br_"), BatchNo: batch.BatchNo, CreatedBy: a.ID, CreatedAt: now()}
		checks := make([]model.Check, 0, len(lines))
		matched := map[string]bool{}
		hashes := make([]string, 0, len(lines))
		fingerprints := make([]string, 0, len(lines))
		for _, line := range lines {
			hash, e := s.payout().BankReconciliationDigest("line", []any{line.ChannelAccountNo, line.ReferenceFingerprint, line.RecipientFingerprint, line.Currency, line.Amount, line.BankStatus, line.BookedAt})
			if e != nil {
				return "", e
			}
			if hash != line.ContentHash || line.ChannelAccountNo != batch.ChannelAccountNo {
				return "", ErrState
			}
			hashes = append(hashes, hash)
			fingerprints = append(fingerprints, line.ReferenceFingerprint)
		}
		sort.Strings(hashes)
		source, e := s.payout().BankReconciliationDigest("batch", []any{batch.ChannelAccountNo, batch.PeriodFrom, batch.PeriodTo, batch.DeclaredComplete, hashes})
		if e != nil {
			return "", e
		}
		if source != batch.SourceHash {
			return "", ErrState
		}
		var bankAttempts []pm.Attempt
		if e = tx.Where("channel_account_no=? AND payment_reference_fingerprint IN ?", batch.ChannelAccountNo, fingerprints).Limit(MaxRows + 1).Find(&bankAttempts).Error; e != nil {
			return "", e
		}
		byReference := map[string]pm.Attempt{}
		orderNos := make([]string, 0, len(bankAttempts))
		for _, at := range bankAttempts {
			if at.PaymentReferenceFingerprint == nil {
				return "", ErrState
			}
			byReference[*at.PaymentReferenceFingerprint] = at
			orderNos = append(orderNos, at.PayoutNo)
		}
		var orders []pm.Order
		if len(orderNos) > 0 {
			if e = tx.Where("payout_no IN ?", orderNos).Limit(MaxRows + 1).Find(&orders).Error; e != nil {
				return "", e
			}
		}
		byOrder := map[string]pm.Order{}
		for _, order := range orders {
			byOrder[order.PayoutNo] = order
		}
		for _, line := range lines {
			c := model.Check{CheckNo: number("bc_"), RunNo: run.RunNo, LineNo: line.LineNo, Result: "unmatched_bank"}
			if at, found := byReference[line.ReferenceFingerprint]; found {
				matched[at.AttemptNo] = true
				c.AttemptNo = at.AttemptNo
				c.PayoutNo = at.PayoutNo
				c.AttemptVersion = at.Version
				order, found := byOrder[at.PayoutNo]
				if !found {
					return "", ErrState
				}
				recipient, e := s.payout().BankOrderRecipientFingerprint(order)
				if e != nil {
					return "", e
				}
				switch {
				case line.Currency != at.Currency || line.Currency != batch.Currency:
					c.Result = "currency_mismatch"
				case line.Amount != at.Amount || at.Amount != order.Amount:
					c.Result = "amount_mismatch"
				case recipient != line.RecipientFingerprint:
					c.Result = "recipient_mismatch"
				case line.BankStatus != "settled":
					c.Result = "bank_" + line.BankStatus
				case at.Status != "succeeded" || order.Status != "succeeded":
					c.Result = "platform_not_succeeded"
				default:
					c.Result = "matched"
				}
			}
			checks = append(checks, c)
		}
		// Bank booking time can differ from payment time: missing is a discrepancy,
		// never proof of failure, and partial imports are explicitly distinguished.
		var attempts []pm.Attempt
		if e := tx.Where("channel_account_no=? AND status='succeeded' AND payment_time>=? AND payment_time<?", batch.ChannelAccountNo, batch.PeriodFrom, batch.PeriodTo).Limit(MaxRows + 1).Find(&attempts).Error; e != nil {
			return "", e
		}
		if len(attempts) > MaxRows {
			return "", ErrLimit
		}
		for _, at := range attempts {
			if matched[at.AttemptNo] {
				continue
			}
			result := "not_seen_partial"
			if batch.DeclaredComplete {
				result = "missing_in_statement"
			}
			checks = append(checks, model.Check{CheckNo: number("bc_"), RunNo: run.RunNo, PayoutNo: at.PayoutNo, AttemptNo: at.AttemptNo, AttemptVersion: at.Version, Result: result})
		}
		for _, c := range checks {
			if c.Result != "matched" {
				run.IssueCount++
			}
		}
		run.CheckCount = len(checks)
		if e := tx.Create(&run).Error; e != nil {
			return "", e
		}
		if len(checks) > 0 {
			if e := tx.CreateInBatches(&checks, 100).Error; e != nil {
				return "", e
			}
		}
		return run.RunNo, nil
	})
}

func (s *Service) Review(ctx context.Context, a payout.Actor, trace string, r req.Review) (Result, error) {
	if !noRE.MatchString(r.CheckNo) || !strings.HasPrefix(r.CheckNo, "bc_") || !safeText(r.Reason, 500) || len(r.Reason) < 3 || (r.Decision != "acknowledge" && r.Decision != "investigate") {
		return Result{}, ErrInvalid
	}
	return s.command(ctx, a, trace, "review", r.RequestID, r, func(tx *gorm.DB) (string, error) {
		var check model.Check
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("check_no=?", r.CheckNo).Take(&check).Error; e != nil {
			return "", ErrState
		}
		var run model.Run
		if e := tx.Where("run_no=?", check.RunNo).Take(&run).Error; e != nil {
			return "", e
		}
		var batch model.Batch
		if e := tx.Where("batch_no=?", run.BatchNo).Take(&batch).Error; e != nil {
			return "", e
		}
		self := a.ID == run.CreatedBy || a.ID == batch.CreatedBy
		if check.AttemptNo != "" {
			var at pm.Attempt
			if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("attempt_no=?", check.AttemptNo).Take(&at).Error; e != nil {
				return "", e
			}
			self = self || at.OperatorID == a.ID
			if at.Version != check.AttemptVersion {
				return "", ErrState
			}
		}
		root := system.IsSuperAdmin(a.ID)
		if self && !root {
			return "", ErrSelfReview
		}
		if r.Decision == "acknowledge" && check.Result != "matched" {
			return "", ErrState
		}
		var old model.Review
		e := tx.Where("check_no=?", check.CheckNo).Take(&old).Error
		if e == nil {
			return "", ErrState
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return "", e
		}
		review := model.Review{ReviewNo: number("bv_"), CheckNo: check.CheckNo, Decision: r.Decision, Reason: redactRE.ReplaceAllString(r.Reason, "[已脱敏]"), ActorID: a.ID, RootException: self && root, CreatedAt: now()}
		return review.ReviewNo, tx.Create(&review).Error
	})
}
