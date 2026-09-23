import service from '@/utils/request'

export const getLiveRoomList = (params) => service({ url: '/live/room/list', method: 'get', params })

export const getLiveRoomDetail = (params) => service({ url: '/live/room/detail', method: 'get', params })

export const updateLiveRoom = (data) => service({ url: '/live/room/update', method: 'post', data })

export const updateLiveRoomStatus = (data) => service({ url: '/live/room/status/update', method: 'post', data })

export const getLiveSessionList = (params) => service({ url: '/live/session/list', method: 'get', params })

export const getLiveSessionDetail = (params) => service({ url: '/live/session/detail', method: 'get', params })

export const endLiveSession = (data) => service({ url: '/live/session/end', method: 'post', data })
