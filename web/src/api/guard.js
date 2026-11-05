import service from '@/utils/request'
/** 守护后台查询；权限由服务端逐请求检查。 */
export const guardQuery = (path, params = {}) => service({ url: `/guard/${path}`, method: 'get', params })
/** 修改必须携带永久 requestId，超时后保留同一请求。 */
export const guardWrite = (path, data) => service({ url: `/guard/${path}`, method: 'post', data })
