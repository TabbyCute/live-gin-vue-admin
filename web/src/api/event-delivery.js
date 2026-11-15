import service from '@/utils/request'
/** root-only durable delivery administration; no mock transport endpoint. */
export const listEventDeliveries = (params) => service({ url: '/event-delivery/list', method: 'get', params })
export const getEventDelivery = (id) => service({ url: '/event-delivery/detail', method: 'get', params: { id } })
export const retryEventDelivery = (data) => service({ url: '/event-delivery/retry', method: 'post', data })
