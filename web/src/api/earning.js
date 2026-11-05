import service from '@/utils/request'

export const earningQuery = (path, params = {}) => service({ url: `/earning/${path}`, method: 'get', params })
export const earningWrite = (path, data) => service({ url: `/earning/${path}`, method: 'post', data })
