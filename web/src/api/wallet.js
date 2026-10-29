import service from '@/utils/request'

// 后台钱包账户
export const getWalletAccount = (params) => service({ url: '/wallet/account/detail', method: 'get', params })
export const setWalletAccountStatus = (data) => service({ url: '/wallet/account/status', method: 'post', data })
export const checkWalletAccount = (params) => service({ url: '/wallet/account/check', method: 'get', params })

// 后台余额流水
export const getWalletLedger = (params) => service({ url: '/wallet/ledger/list', method: 'get', params })

// 系统人工充值单
export const getWalletAdjustmentList = (params) => service({ url: '/wallet/adjustment/list', method: 'get', params })
export const getWalletAdjustmentDetail = (params) => service({ url: '/wallet/adjustment/detail', method: 'get', params })
export const createWalletRecharge = (data) => service({ url: '/wallet/adjustment/create', method: 'post', data })
export const approveWalletAdjustment = (data) => service({ url: '/wallet/adjustment/approve', method: 'post', data })
export const rejectWalletAdjustment = (data) => service({ url: '/wallet/adjustment/reject', method: 'post', data })
export const reverseWalletAdjustment = (data) => service({ url: '/wallet/adjustment/reverse', method: 'post', data })
