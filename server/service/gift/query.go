package gift

import (
	"context"
	"database/sql"
	"errors"
	"gorm.io/gorm"
	"strconv"
	earning "tb_live_module/model/earning"
	model "tb_live_module/model/gift"
	req "tb_live_module/model/gift/request"
	res "tb_live_module/model/gift/response"
	"time"
)

func page(q *req.Query) error {
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}
	if q.Page < 1 || q.Page > 10000 || q.PageSize < 1 || q.PageSize > 100 || len(q.Name) > 64 {
		return ErrInvalid
	}
	return nil
}
func priceFilter(db *gorm.DB, min, max, column string) (*gorm.DB, error) {
	if min != "" {
		v, e := integer(min, true)
		if e != nil {
			return nil, e
		}
		db = db.Where(column+">=?", v)
	}
	if max != "" {
		v, e := integer(max, true)
		if e != nil {
			return nil, e
		}
		db = db.Where(column+"<=?", v)
	}
	return db, nil
}
func orderBy(q req.Query, allowed map[string]string, def string) (string, error) {
	if q.Sort == "" {
		return def, nil
	}
	s, ok := allowed[q.Sort]
	if !ok {
		return "", ErrInvalid
	}
	if q.Desc {
		s += " DESC"
	} else {
		s += " ASC"
	}
	return s + ", id ASC", nil
}
func (s *Service) Catalog(ctx context.Context, user uint64, a *Actor, kind string, q req.Query) (res.Page, error) {
	var out res.Page
	if e := page(&q); e != nil {
		return out, e
	}
	out.Page = q.Page
	out.PageSize = q.PageSize
	e := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		if a != nil {
			if e := s.authorize(tx, *a, kind+"/list", "GET"); e != nil {
				return e
			}
		} else if user == 0 {
			return ErrForbidden
		}
		if a == nil && q.RoomNo != "" {
			if _, _, _, e := validRoom(tx, user, q.RoomNo); e != nil {
				return e
			}
		}
		var c model.Control
		if e := tx.First(&c, 1).Error; e != nil {
			return e
		}
		out.Version = strconv.FormatInt(c.Version, 10)
		switch kind {
		case "category":
			db := tx.Model(&model.Category{})
			if a == nil {
				db = db.Where("status='enabled'")
			} else if q.Status != "" {
				db = db.Where("status=?", q.Status)
			}
			if q.Name != "" {
				db = db.Where("name LIKE ?", "%"+q.Name+"%")
			}
			if e := db.Count(&out.Total).Error; e != nil {
				return e
			}
			var list []model.Category
			e := db.Order("sort_order ASC,id ASC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Find(&list).Error
			out.List = list
			return e
		case "item":
			db := tx.Model(&model.Item{})
			if a == nil {
				db = db.Where("status='enabled' AND category_no IN (SELECT category_no FROM gift_category WHERE status='enabled')")
			} else if q.Status != "" {
				db = db.Where("status=?", q.Status)
			}
			if q.CategoryNo != "" {
				db = db.Where("category_no=?", q.CategoryNo)
			}
			if q.Name != "" {
				db = db.Where("name LIKE ?", "%"+q.Name+"%")
			}
			var e error
			db, e = priceFilter(db, q.MinAmount, q.MaxAmount, "coin_price")
			if e != nil {
				return e
			}
			sort, e := orderBy(q, map[string]string{"price": "coin_price", "sortOrder": "sort_order"}, "sort_order ASC,id ASC")
			if e != nil {
				return e
			}
			if e = db.Count(&out.Total).Error; e != nil {
				return e
			}
			var list []model.Item
			e = db.Order(sort).Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Find(&list).Error
			out.List = list
			return e
		default:
			return ErrInvalid
		}
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	return out, e
}
func filters(db *gorm.DB, q req.Query) (*gorm.DB, error) {
	for _, f := range []struct{ col, v string }{{"order_no", q.No}, {"sender_account_no", q.AccountNo}, {"anchor_no", q.AnchorNo}, {"room_no", q.RoomNo}, {"session_no", q.SessionNo}, {"gift_no", q.GiftNo}, {"category_no", q.CategoryNo}, {"status", q.Status}} {
		if f.v != "" {
			if len(f.v) > 64 {
				return nil, ErrInvalid
			}
			db = db.Where(f.col+"=?", f.v)
		}
	}
	if q.From < 0 || q.To < 0 || (q.To > 0 && q.From >= q.To) {
		return nil, ErrInvalid
	}
	if q.From > 0 {
		db = db.Where("created_at>=?", q.From)
	}
	if q.To > 0 {
		db = db.Where("created_at<?", q.To)
	}
	if q.Refunded == "yes" {
		db = db.Where("refunded_coin_amount>0")
	} else if q.Refunded == "no" {
		db = db.Where("refunded_coin_amount=0")
	} else if q.Refunded != "" {
		return nil, ErrInvalid
	}
	return priceFilter(db, q.MinAmount, q.MaxAmount, "gross_coin_amount")
}
func (s *Service) Orders(ctx context.Context, user uint64, a *Actor, q req.Query) (res.Page, error) {
	var out res.Page
	if e := page(&q); e != nil {
		return out, e
	}
	out.Page = q.Page
	out.PageSize = q.PageSize
	e := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		db := tx.Model(&model.Order{})
		if a != nil {
			if e := s.authorize(tx, *a, "order/list", "GET"); e != nil {
				return e
			}
		} else {
			if user == 0 {
				return ErrForbidden
			}
			db = db.Where("sender_user_id=?", user)
		}
		if q.RequestID != "" {
			if !keyPattern.MatchString(q.RequestID) {
				return ErrInvalid
			}
			db = db.Where("request_id=?", q.RequestID)
		}
		db, e := filters(db, q)
		if e != nil {
			return e
		}
		sort, e := orderBy(q, map[string]string{"createdAt": "created_at", "amount": "gross_coin_amount", "refund": "refunded_coin_amount", "quantity": "quantity"}, "id DESC")
		if e != nil {
			return e
		}
		if e = db.Count(&out.Total).Error; e != nil {
			return e
		}
		var list []model.Order
		e = db.Order(sort).Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Find(&list).Error
		out.List = list
		if a == nil {
			public := make([]res.AppOrder, 0, len(list))
			for _, o := range list {
				public = append(public, res.PublicOrder(o))
			}
			out.List = public
		}
		return e
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, e
}
func (s *Service) Result(ctx context.Context, user uint64, q req.Query) (model.Order, error) {
	var o model.Order
	if user == 0 || (q.No == "" && q.RequestID == "") || (q.No != "" && q.RequestID != "") {
		return o, ErrInvalid
	}
	db := s.db(ctx).Where("sender_user_id=?", user)
	if q.No != "" {
		db = db.Where("order_no=?", q.No)
	} else {
		db = db.Where("request_id=?", q.RequestID)
	}
	e := db.Take(&o).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		e = ErrNotFound
	}
	return o, e
}
func (s *Service) Detail(ctx context.Context, a Actor, q req.Query) (res.Detail, error) {
	var out res.Detail
	e := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		if e := s.authorize(tx, a, "order/detail", "GET"); e != nil {
			return e
		}
		if e := tx.Where("order_no=?", q.No).Take(&out.Order).Error; e != nil {
			return ErrNotFound
		}
		if e := loadOrderFacts(tx, &out); e != nil {
			return e
		}
		for _, p := range []struct {
			dest   any
			column string
		}{{&out.Benefits, "gift_order_no"}, {&out.Compensations, "gift_order_no"}} {
			if e := tx.Where(p.column+"=?", q.No).Order("id ASC").Limit(10002).Find(p.dest).Error; e != nil {
				return e
			}
		}
		if e := checkOrderFacts(tx, &out); e != nil {
			return e
		}
		if s.EnrichEarnings != nil {
			return s.EnrichEarnings(tx, out.Earnings)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, e
}
func shareRemaining(o model.Order) int64 {
	return (o.GrossCoinAmount-o.RefundedCoinAmount)/10000*o.AnchorShareBPS + (o.GrossCoinAmount-o.RefundedCoinAmount)%10000*o.AnchorShareBPS/10000
}
func (s *Service) Rules(ctx context.Context, a Actor, q req.Query) (res.Page, error) {
	var out res.Page
	if e := page(&q); e != nil {
		return out, e
	}
	out.Page = q.Page
	out.PageSize = q.PageSize
	e := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		if e := s.authorize(tx, a, "rule/list", "GET"); e != nil {
			return e
		}
		db := tx.Model(&earning.RevenueRule{})
		if q.Status != "" {
			db = db.Where("status=?", q.Status)
		}
		if e := db.Count(&out.Total).Error; e != nil {
			return e
		}
		var list []earning.RevenueRule
		e := db.Order("id DESC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Find(&list).Error
		if e != nil {
			return e
		}
		rows := make([]res.Rule, 0, len(list))
		numbers := make([]string, 0, len(list))
		for _, r := range list {
			numbers = append(numbers, r.RuleNo)
		}
		var decisions []earning.RuleTermination
		if len(numbers) > 0 {
			if e := tx.Where("id IN (SELECT MAX(id) FROM anchor_rule_termination WHERE rule_no IN ? GROUP BY rule_no)", numbers).Find(&decisions).Error; e != nil {
				return e
			}
		}
		latest := map[string]*earning.RuleTermination{}
		for i := range decisions {
			latest[decisions[i].RuleNo] = &decisions[i]
		}
		for _, r := range list {
			rows = append(rows, res.Rule{RevenueRule: r, Termination: latest[r.RuleNo]})
		}
		out.List = rows
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, e
}
func (s *Service) Ranking(ctx context.Context, a Actor, q req.Query) (res.Page, error) {
	var out res.Page
	if e := page(&q); e != nil {
		return out, e
	}
	out.Page = q.Page
	out.PageSize = q.PageSize
	now := time.UnixMilli(s.now()).UTC()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	switch q.Period {
	case "day":
		q.From = day.UnixMilli()
		q.To = day.AddDate(0, 0, 1).UnixMilli()
	case "week":
		day = day.AddDate(0, 0, -(int(day.Weekday())+6)%7)
		q.From = day.UnixMilli()
		q.To = day.AddDate(0, 0, 7).UnixMilli()
	case "month":
		day = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		q.From = day.UnixMilli()
		q.To = day.AddDate(0, 1, 0).UnixMilli()
	case "custom", "":
	default:
		return out, ErrInvalid
	}
	if q.From <= 0 || q.To <= q.From || q.To-q.From > 93*86400000 {
		return out, ErrInvalid
	}
	columns := map[string]string{"sender": "sender_account_no", "anchor": "anchor_no", "room": "room_no", "session": "session_no", "gift": "gift_no", "category": "category_no", "platform": "'platform'"}
	dimension, ok := columns[q.Dimension]
	if !ok {
		return out, ErrInvalid
	}
	sort := q.Sort
	if sort == "" {
		sort = "net"
	}
	allowed := map[string]string{"net": "SUM(gross_coin_amount-refunded_coin_amount)", "gross": "SUM(gross_coin_amount)", "refund": "SUM(refunded_coin_amount)", "quantity": "SUM(quantity-refunded_coin_amount/unit_price)", "orders": "COUNT(*)", "users": "COUNT(DISTINCT CASE WHEN gross_coin_amount>refunded_coin_amount THEN sender_user_id END)", "earning": "SUM(estimated_anchor_earning_coin)"}
	order, ok := allowed[sort]
	if !ok {
		return out, ErrInvalid
	}
	e := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		if e := s.authorize(tx, a, "ranking", "GET"); e != nil {
			return e
		}
		db, e := filters(tx.Model(&model.Order{}), q)
		if e != nil {
			return e
		}
		div := "/"
		if tx.Dialector.Name() == "mysql" {
			div = " DIV "
		}
		net := "(gross_coin_amount-refunded_coin_amount)"
		earn := "((" + net + div + "10000)*anchor_share_bps+(" + net + "%10000)*anchor_share_bps" + div + "10000)"
		if sort == "earning" {
			order = "SUM(" + earn + ")"
		}
		grouped := db.Session(&gorm.Session{}).Select(dimension + " AS no").Group(dimension)
		if e = tx.Table("(?) AS gift_groups", grouped).Count(&out.Total).Error; e != nil {
			return e
		}
		selectSQL := dimension + " AS no, CAST(SUM(gross_coin_amount) AS CHAR) AS gross, CAST(SUM(refunded_coin_amount) AS CHAR) AS refund, CAST(SUM(" + net + ") AS CHAR) AS net, CAST(SUM(" + earn + ") AS CHAR) AS earning, CAST(SUM(quantity-refunded_coin_amount" + div + "unit_price) AS CHAR) AS quantity, CAST(COUNT(*) AS CHAR) AS orders, CAST(COUNT(DISTINCT CASE WHEN gross_coin_amount>refunded_coin_amount THEN sender_user_id END) AS CHAR) AS users"
		direction := " DESC"
		if q.Sort != "" && !q.Desc {
			direction = " ASC"
		}
		var rows []res.Ranking
		e = db.Select(selectSQL).Group(dimension).Order(order + direction + ", no ASC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Scan(&rows).Error
		out.List = rows
		return e
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, e
}
