import service from '@/utils/request'

/** Query a visible page of rooms/sessions. Live counts are unknown when stale=true. */
export const getLivePresenceStats = (data) => service({
  url: '/live/presence/stats', method: 'post',
  data: { roomNos: data.roomNos || [], sessionNos: data.sessionNos || [] },
  timeout: 6000, donNotShowLoading: true
})
