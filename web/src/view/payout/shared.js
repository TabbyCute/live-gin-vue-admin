export const canSubmitPayout = row => row?.status === 'approved' && row.reconciliationStatus === 'passed' && /^pa_[a-f0-9]{32}$/.test(row.payeeRef || '') && !row.payoutNo
export const payoutError = error => ({49201:'提交参数无效，请刷新页面核对',49202:'没有打款中心操作权限',49204:'请求号与原提现单冲突，请核对原提交结果',49205:'提现、收款账户或账务核验未通过，请查看提现详情',49206:'出款订单不存在',49107:'操作频繁，请稍后用原请求重试'}[Number(error?.code)] || '暂时无法确认结果，请按原参数重试或查询该提现单对应的出款订单')
export const orderFields = [['payoutNo','出款单号'],['businessType','来源业务'],['businessNo','提现单号'],['anchorNo','主播编号'],['amount','金额（最小单位）'],['currency','币种'],['payeeMethod','收款方式'],['accountMasked','脱敏收款账户'],['payeeAccountNo','收款账户编号'],['payeeVersion','账户版本'],['status','状态'],['createdBy','提交人'],['createdAt','创建时间']]
export const payoutStates = { ready:'待领取', processing:'人工处理中', pending_confirmation:'待独立复核', succeeded:'人工确认付款成功', failed:'明确失败', unknown:'结果未知，继续冻结', cancelled:'已取消并解冻' }
export const manualActions = [
 ['order/start','领取并开始人工出款'],['order/retry','失败后重新领取'],['order/release','未转账释放领取'],['attempt/fail-unpaid','确认尚未转账失败'],['attempt/bank-submitted','标记已提交银行'],['attempt/submit','提交真实流水和凭证'],['attempt/review-success','独立复核成功'],['attempt/review-failure','独立复核明确失败'],['attempt/review-unknown','标记结果未知'],['order/investigate','追加调查记录'],['attempt/resolve-unknown','解决未知结果'],['order/cancel','确认未付款取消提现'],['order/reconcile','运行内部对账']
]
export function manualBody(path, form, detail, requestId) {
 const attemptNo = detail.attempts?.at(-1)?.attemptNo, payoutNo = detail.order.payoutNo
 const body = { requestId }
 if (['order/start','order/retry'].includes(path)) return { ...body, payoutNo, channelAccountNo:form.channelAccountNo, reason:form.reason }
 if (['order/release','attempt/fail-unpaid'].includes(path)) return { ...body, attemptNo, reason:form.reason, confirmNoTransfer:form.confirm }
 if (path==='attempt/submit') return { ...body, attemptNo, paymentReference:form.paymentReference, paymentTime:Number(form.paymentTime), evidenceNo:form.evidenceNo, operatorNote:form.reason, confirmRealTransfer:form.confirm }
 if (path.startsWith('attempt/review-')) return { ...body, attemptNo, reason:form.reason, confirmExternalResult:form.confirm }
 if (path==='attempt/resolve-unknown') return { ...body, attemptNo, investigationNo:form.investigationNo, decision:form.decision, reason:form.reason, confirmExternalResult:form.confirm }
 if (path==='order/investigate') return { ...body, payoutNo, note:form.reason, evidenceNo:form.evidenceNo }
 if (path==='order/cancel') return { ...body, payoutNo, reason:form.reason, confirmNoTransfer:form.confirm }
 if (path==='attempt/bank-submitted') return { ...body, attemptNo, reason:form.reason }
 if (path==='order/reconcile') return { ...body, payoutNo, reason:form.reason }
 throw new Error('Unsupported payout action')
}
export const manualError = e => ({49207:'操作人与复核人必须不同；仅 ID=1 的超级 root 可按审计例外处理',49208:'查看凭证已过期或已使用，请重新申请',49209:'私有凭证不可用、格式不符或无权访问',49210:'公司账户不可用或超过平台风控限额',49211:'解决未知结果必须提供独立调查和新的私有凭证'}[Number(e?.code)] || payoutError(e))
