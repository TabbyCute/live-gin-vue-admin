import service from '@/utils/request'
/** 榜单只读查询，不修改礼物或钱包。 */
export const rankingQuery = (path, params = {}) => service({ url: `/ranking/admin/${path}`, method: 'get', params })
/** 受控重建或核验。 */
export const rankingWrite = (path, data) => service({ url: `/ranking/admin/${path}`, method: 'post', data })
