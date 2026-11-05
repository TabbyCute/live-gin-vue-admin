// Settlement timestamps use milliseconds on the wire and whole seconds in the UI.
export const currentSecond = (now = Date.now()) => Math.floor(now / 1000) * 1000
export const recentSettlementPeriod = (now = Date.now()) => {
  const end = currentSecond(now)
  return { periodStart: end - 7 * 24 * 60 * 60 * 1000, periodEnd: end }
}
export const usableRule = (rule, now = Date.now()) => Boolean(rule && rule.status === 'active' && rule.sourceUnit === 'COIN' && rule.effectiveFrom <= now && rule.effectiveTo > now)
export function settlementPayload(draft, rule, now = Date.now()) {
  const data = { requestId: draft.requestId, anchorNo: draft.anchorNo, ruleNo: draft.ruleNo, periodStart: Number(draft.periodStart), periodEnd: Number(draft.periodEnd) }
  if (!/^[A-Za-z0-9_-]{1,40}$/.test(data.anchorNo || '')) throw new Error('请填写有效的主播编号')
  if (!rule || data.ruleNo !== rule.ruleNo || !usableRule(rule, now)) throw new Error('请选择当前已生效的兑换规则，过期规则请重新选择')
  if (draft.periodStart === '' || draft.periodStart == null || draft.periodEnd === '' || draft.periodEnd == null || !Number.isSafeInteger(data.periodStart) || !Number.isSafeInteger(data.periodEnd) || data.periodStart < 0 || data.periodStart >= data.periodEnd) throw new Error('请选择有效的结算开始、结束时间')
  if (data.periodEnd > currentSecond(now)) throw new Error('结算结束时间不能晚于当前时分秒')
  return data
}
const after = (max, value) => Array.from({ length: max }, (_, i) => i).filter(i => i > value)
export function futureTimes(value, now = Date.now()) {
  const day = new Date(Number(value)), today = new Date(now)
  const sameDay = value !== '' && value != null && day.toDateString() === today.toDateString()
  return {
    hours: () => sameDay ? after(24, today.getHours()) : [],
    minutes: hour => sameDay && hour === today.getHours() ? after(60, today.getMinutes()) : [],
    seconds: (hour, minute) => sameDay && hour === today.getHours() && minute === today.getMinutes() ? after(60, today.getSeconds()) : []
  }
}
