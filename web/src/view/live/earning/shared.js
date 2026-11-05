export const integerAmount = value => typeof value === 'string' && /^[1-9][0-9]{0,18}$/.test(value) && (value.length < 19 || value <= '9223372036854775807')
export const exchangePreview = (coinUnits, minorUnits, currency, sampleCoin = '600') => {
  if (!integerAmount(coinUnits) || !integerAmount(minorUnits) || !/^[A-Z]{3,8}$/.test(currency) || !/^[1-9][0-9]*$/.test(sampleCoin)) return ''
  const amount = BigInt(sampleCoin) * BigInt(minorUnits) / BigInt(coinUnits)
  if (currency === 'CNY') {
    const hundred = BigInt(100)
    const yuan = amount / hundred
    const cent = (amount % hundred).toString().padStart(2, '0')
    return `${sampleCoin} 收益 COIN → ${yuan}.${cent} CNY（${amount} 分）`
  }
  return `${sampleCoin} 收益 COIN → ${amount} ${currency} 最小单位`
}
export const statusLabel = value => ({ pending: '待复核', active: '生效', approved: '审核通过', rejected: '已驳回', pending_review: '待审核', paid:'人工复核已付款', cancelled:'已取消并解冻', passed: '对账通过', abnormal: '对账异常', income: '收益', reversal: '退款冲减', settlement_credit: '结算入账', negative_adjustment: '负向调整', withdrawal_freeze: '提现冻结', withdrawal_release: '提现冻结释放', withdrawal_paid:'出款成功入账' }[value] || value || '—')
export const sourceLabel = value => ({ gift: '直播礼物', bounty: '大赏', donation: '赠送', guard: '守护', fan_club: '粉丝团', charge: '充电' }[value] || value)
export const sequence = () => { let version = 0; return { next: () => ++version, valid: value => value === version, invalidate: () => ++version } }
export const snapshot = value => { try { return JSON.parse(value || '{}') } catch { return { consistent: false, issues: ['快照内容无法解析，请联系技术人员核查'] } } }
export const columns = {
 income: [['incomeNo','收益编号'],['sourceType','来源'],['sourceNo','业务原单'],['entryType','类型'],['amount','主播收益 COIN'],['grossAmount','原始收入 COIN'],['createdAt','发生时间']],
 exchange: [['ruleNo','规则编号'],['name','名称'],['sourceUnit','收益单位'],['currency','结算币种'],['coinUnits','每档收益 COIN'],['minorUnits','每档兑换金额（最小单位）'],['status','状态'],['effectiveFrom','生效时间'],['effectiveTo','截止时间']],
 settlement: [['settlementNo','结算单号'],['anchorNo','主播'],['currency','币种'],['earningCoin','结算收益 COIN'],['amount','结算金额（最小单位）'],['status','状态'],['createdAt','创建时间']],
 account: [['anchorNo','主播'],['currency','币种'],['available','可提现（最小单位）'],['frozen','冻结'],['debt','待抵扣'],['version','账务版本'],['updatedAt','更新时间']],
 withdrawal: [['withdrawalNo','提现单号'],['anchorNo','主播'],['currency','币种'],['amount','申请金额（最小单位）'],['payeeMethod','收款方式'],['accountMasked','申请时收款账户'],['payeeVersion','收款账户版本'],['status','状态'],['reconciliationStatus','对账状态'],['createdAt','申请时间']],
 reconciliation: [['reconciliationNo','对账编号'],['withdrawalNo','提现单号'],['currency','币种'],['status','核验结果'],['createdAt','核验时间']]
}
columns.withdrawal.splice(7,0,['payeeRef','收款账户编号'])
export const labels = Object.fromEntries(Object.values(columns).flat())
Object.assign(labels,{payoutNo:'出款单号',payoutStatus:'出款状态'})
Object.assign(labels, { opening:'期初余额',settlementIncome:'结算收入',negativeAdjustments:'退款与负向调整',manualAdjustments:'人工调整',paid:'已成功提现',available:'可提现余额',frozen:'冻结金额',debt:'待抵扣金额',version:'账户版本',requestedAmount:'本次申请',anchorHealthy:'主播资格通过',payeeChanged:'收款账户已变更',consistent:'账务一致',checkedAt:'核验时间',periodStart:'结算开始',periodEnd:'结算结束',grossCoin:'原始收入 COIN',refundCoin:'退款 COIN',ruleNo:'兑换规则',reviewReason:'审核原因',accountName:'收款人资料展示',payeeMethod:'收款方式',payeeRef:'收款账户编号',payeeVersion:'申请时账户版本',reviewedBy:'审核人',approvedBy:'复核人',approvedAt:'复核时间',reviewedAt:'审核时间',createdBy:'创建人',itemCount:'收益笔数',changeAmount:'发生金额',availableBefore:'变动前可用',availableAfter:'变动后可用',frozenBefore:'变动前冻结',frozenAfter:'变动后冻结',debtBefore:'变动前欠额',debtAfter:'变动后欠额',ledgerNo:'流水号',businessNo:'业务单号',action:'操作',actorId:'操作人',actorType:'操作者类型',actorIp:'操作 IP',snapshotHash:'快照校验值',incomeHash:'收益事实校验值' })

export const payeeMethodLabel = value => ({bank_card:'银行卡',alipay:'支付宝',wechat:'微信收款资料',payout_center:'历史占位资料'}[value] || value || '—')
export const payeeStateLabel = value => ({active:'已绑定（未做渠道认证）',disabled:'已停用',missing:'账户缺失',legacy_placeholder:'历史占位单：禁止出款',unverifiable:'资料完整性校验失败',unavailable:'当前资料不可用'}[value] || value || '—')
