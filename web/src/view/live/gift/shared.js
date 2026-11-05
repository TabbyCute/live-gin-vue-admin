export const positiveCoin = (value) => /^[1-9][0-9]{0,18}$/.test(value) && BigInt(value) <= BigInt('9223372036854775807')
export const refundAmount = (order, quantity) => {
  if (!positiveCoin(quantity)) throw new Error('请输入正整数件数')
  const amount = BigInt(order.unitPrice) * BigInt(quantity)
  if (amount > BigInt(order.grossCoinAmount) - BigInt(order.refundedCoinAmount)) throw new Error('超过可退款数量')
  return amount.toString()
}
export const fields = {
  category: [ ['categoryNo', '分类编号'], ['name', '名称'], ['iconUrl', '图标 HTTPS 地址'], ['description', '说明'] ],
  item: [ ['giftNo', '礼物编号'], ['categoryNo', '分类编号'], ['name', '名称'], ['iconUrl', '图标 HTTPS 地址'], ['animationUrl', '动画 HTTPS 地址'], ['coinPrice', '单价 COIN'], ['maxQuantityPerOrder', '每单最多件数'], ['description', '说明'], ['guardPlanNo', '守护计划编号'], ['guardDurationDays', '每件守护天数'] ]
}
export const emptyForm = (kind) => kind === 'category'
  ? { categoryNo: '', name: '', iconUrl: '', description: '', sortOrder: 0, status: 'enabled', version: '' }
  : { giftNo: '', categoryNo: '', name: '', iconUrl: '', animationUrl: '', animationType: 'none', coinPrice: '', sortOrder: 0, status: 'draft', isPremium: false, maxQuantityPerOrder: '1', description: '', benefitType: 'none', guardPlanNo: '', guardDurationDays: '', benefitStatus: 'disabled', effectiveFrom: 0, effectiveTo: 0, version: '' }
export const editForm = (kind, row) => Object.fromEntries(Object.keys(emptyForm(kind)).map(key => [key, row[key] ?? emptyForm(kind)[key]]))
// Latest request wins; cancellation is not relied upon for UI correctness.
export const requestSequence = () => { let current = 0; return { next: () => ++current, valid: value => value === current, invalidate: () => ++current } }
