export const labels = { enabled: '启用', disabled: '停用', active: '生效', expired: '已到期', pending: '等待处理', processing: '处理中', failed: '失败待重试', succeeded: '已完成', compensated: '已撤回', grant: '首次开通', renew: '续期', compensate: '退款补偿', expire: '到期处理', rebuild: '重建' }
export const emptyPlan = () => ({ planNo: '', name: '', level: 1, priority: 10, defaultDurationDays: '30', benefitConfig: '{}', status: 'disabled', sortOrder: 0, version: '' })
export const planForm = row => Object.fromEntries(Object.entries(emptyPlan()).map(([key, value]) => [key, row?.[key] ?? value]))
// Freeze the complete payload; a lost response must never turn retry into a new operation.
export const actionAttempt = (requestId, payload) => Object.freeze({ ...payload, requestId })
// Element Plus needs a number only for its bounded page control. Keep the real
// count as a decimal string; the API itself limits navigation to 10000 pages.
export const pageTotal = (value, pageSize = 20) => {
  if (typeof value !== 'string' || !/^(0|[1-9]\d*)$/.test(value) || value.length > 65) return 0
  const count = BigInt(value), limit = BigInt(pageSize) * 10000n
  return Number(count < limit ? count : limit)
}
