import service from '@/utils/request'
const root = '/payout/bank-reconciliation'
export const listBankBatches = params => service({ url: `${root}/list`, method: 'get', params })
export const getBankBatch = batchNo => service({ url: `${root}/detail`, method: 'get', params: { batchNo } })
export const bankAction = (action, data) => {
  if (!['import', 'run', 'review'].includes(action)) return Promise.reject(new Error('Unsupported bank reconciliation action'))
  return service({ url: `${root}/${action}`, method: 'post', data, timeout: 18000, donNotShowLoading: true })
}
