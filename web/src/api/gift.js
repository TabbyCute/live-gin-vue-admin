import service from '@/utils/request'
const root = '/gift/'
export const giftQuery = (path, params) => service({ url: root + path, method: 'get', params })
export const giftWrite = (path, data) => service({ url: root + path, method: 'post', data })
