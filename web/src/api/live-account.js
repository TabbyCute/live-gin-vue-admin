import service from '@/utils/request'

export const getLiveAccountList = (params) => service({ url: '/live/account/list', method: 'get', params })

export const getLiveAccountDetail = (params) => service({ url: '/live/account/detail', method: 'get', params })

export const updateLiveAccountProfile = (data) => service({ url: '/live/account/profile/update', method: 'post', data })

export const updateLiveAccountStatus = (data) => service({ url: '/live/account/status/update', method: 'post', data })


export const logoutLiveAccountAll = (data) => service({ url: '/live/account/logout-all', method: 'post', data })
