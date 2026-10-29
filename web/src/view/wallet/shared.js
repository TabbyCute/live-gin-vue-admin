import { formatDate } from '@/utils/format'

export const formatWalletTime = (value) => value ? (formatDate(new Date(Number(value))) || '-') : '-'

export const formatWalletAmount = (value) => {
  const text = String(value ?? '0')
  const negative = text.startsWith('-')
  const digits = negative ? text.slice(1) : text
  const formatted = digits.replace(/\B(?=(\d{3})+(?!\d))/g, ',')
  return `${negative ? '-' : ''}${formatted}`
}

export const statusMeta = (status) => ({
  pending: { label: '待审核', type: 'warning' },
  credited: { label: '已入账', type: 'success' },
  rejected: { label: '已驳回', type: 'info' },
  reversed: { label: '已冲正', type: 'danger' }
}[status] || { label: status || '-', type: 'info' })

export const kindLabel = (kind) => kind === 'reverse' ? '冲正' : '充值'
export const sourceLabel = (source) => ({ manual: '人工充值', legacy: '历史数据', reversal: '内部冲正' }[source] || source || '-')
export const businessLabel = (type) => ({ ADMIN_GRANT: '人工充值', ADMIN_REVERSE: '全额冲正' }[type] || type || '-')

export const issueLabel = (issue) => {
  if (issue === 'wallet_balance_invalid') return '钱包余额或冻结值不合法'
  if (issue === 'previous_ledger_missing') return '上一版本流水缺失'
  if (issue === 'wallet_tail_mismatch') return '钱包余额或版本与末条流水不一致'
  const [type, no] = String(issue).split(':', 2)
  const labels = {
    ledger_chain: '流水版本链断裂', ledger_amount: '流水金额不合法', ledger_order: '流水与业务单不一致',
    order_ledger: '业务单与流水不一致', order_audit: '订单审计缺失、重复或与申请/审核事实不一致', order_request_hash: '申请内容摘要异常', legacy_source_unverified: '历史资金来源尚未核验'
  }
  return `${labels[type] || issue}${no ? `：${no}` : ''}`
}
