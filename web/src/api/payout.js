import service from '@/utils/request'

/** Manual bank operations: every mutation requires a permanent requestId. */
export const createPayoutOrder = data => service({ url: '/payout/order/create', method: 'post', data })
export const listPayoutOrders = params => service({ url: '/payout/order/list', method: 'get', params })
export const getPayoutOrder = payoutNo => service({ url: '/payout/order/detail', method: 'get', params: { payoutNo } })
export const getPayoutCapabilities = () => service({ url: '/payout/capabilities', method: 'get', timeout: 10000 })
export const listCompanyAccounts = params => service({ url: '/payout/channel-account/list', method: 'get', params, timeout: 10000 })
const actions = new Set(['channel-account/create','channel-account/enable','channel-account/disable','channel-account/reveal-grant','channel-account/reveal','order/start','order/retry','order/release','order/reveal-payee-grant','order/reveal-payee','order/cancel','order/investigate','order/reconcile','attempt/bank-submitted','attempt/fail-unpaid','attempt/submit','attempt/review-success','attempt/review-failure','attempt/review-unknown','attempt/resolve-unknown','evidence/upload'])
export const manualPayout = (path, data) => {
  if (!actions.has(path)) return Promise.reject(new Error('Unsupported payout action'))
  return service({ url: `/payout/${path}`, method: 'post', data, timeout: path === 'evidence/upload' ? 18000 : 10000, donNotShowLoading: true })
}
export async function readPayoutEvidence(data) {
  const r = await service({ url: '/payout/evidence/read', method: 'post', data, responseType: 'blob', timeout: 10000, donNotShowLoading: true })
  if (!(r.data instanceof Blob) || !['image/png','image/jpeg'].includes(r.data.type)) {
    let code = 49209
    try { code = JSON.parse(await r.data.text()).code || code } catch { /* Never display raw payloads. */ }
    throw { code }
  }
  return r.data
}
