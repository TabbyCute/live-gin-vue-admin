package payout

import (
	"context"
	"gorm.io/gorm"
	"strings"
	em "tb_live_module/model/earning"
	"tb_live_module/model/system"
)

type Operation struct{ Path, Method, Description string }

var ManualOperations = []Operation{
	{"channel-account/list", "GET", "查询公司出款账户"}, {"channel-account/create", "POST", "创建公司出款账户"}, {"channel-account/enable", "POST", "启用公司出款账户"}, {"channel-account/disable", "POST", "停用公司出款账户"}, {"channel-account/reveal-grant", "POST", "申请公司账号查看凭证"}, {"channel-account/reveal", "POST", "单次查看公司完整账号"},
	{"order/start", "POST", "领取出款"}, {"order/retry", "POST", "失败后重新领取"}, {"order/release", "POST", "未转账释放领取"}, {"order/reveal-payee-grant", "POST", "申请收款资料查看凭证"}, {"order/reveal-payee", "POST", "单次查看收款资料"}, {"order/cancel", "POST", "确认未付款取消提现"}, {"order/investigate", "POST", "记录未知结果调查"}, {"order/reconcile", "POST", "运行内部账务对账"},
	{"attempt/bank-submitted", "POST", "标记已提交银行"}, {"attempt/fail-unpaid", "POST", "确认未执行转账"}, {"attempt/submit", "POST", "提交真实转账结果"}, {"attempt/review-success", "POST", "独立复核成功"}, {"attempt/review-failure", "POST", "独立复核明确失败"}, {"attempt/review-unknown", "POST", "标记未知结果"}, {"attempt/resolve-unknown", "POST", "依据新调查解决未知结果"},
	{"evidence/upload", "POST", "上传私有支付凭证"}, {"evidence/read", "POST", "审计查看私有支付凭证"},
}

func (s *Service) capabilitiesInTx(tx *gorm.DB, a Actor) (map[string]bool, error) {
	out := map[string]bool{}
	if e := s.authorizePayoutActor(tx, a); e != nil {
		return out, e
	}
	ops := append([]Operation{{"order/create", "POST", ""}, {"order/list", "GET", ""}, {"order/detail", "GET", ""}}, ManualOperations...)
	if system.IsSuperAdmin(a.ID) {
		for _, op := range ops {
			out[op.Path] = true
		}
		return out, nil
	}
	var rows []em.AdminPermission
	if e := tx.Where("authority_id=? AND path LIKE '/payout/%'", a.AuthorityID).Find(&rows).Error; e != nil {
		return out, e
	}
	for _, op := range ops {
		for _, row := range rows {
			if row.Method == op.Method && strings.TrimPrefix(row.Path, "/payout/") == op.Path {
				out[op.Path] = true
			}
		}
	}
	return out, nil
}
func (s *Service) Capabilities(ctx context.Context, a Actor) (map[string]bool, error) {
	return s.capabilitiesInTx(s.db(ctx), a)
}
