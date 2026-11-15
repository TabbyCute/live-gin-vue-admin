import service from '@/utils/request'

const base = '/v1/admin/anchor-home/album'

/** Authenticated moderation API; media credentials must never enter a URL. */
export const listAnchorPhotos = (data) => service({ url: `${base}/list`, method: 'post', data, timeout: 15000, donNotShowLoading: true })
export const reviewAnchorPhoto = (data) => service({ url: `${base}/review`, method: 'post', data, timeout: 15000, donNotShowLoading: true })
export const previewAnchorPhoto = async (photoNo) => {
  const response = await service({ url: `${base}/preview`, method: 'get', params: { photoNo, variant: 'full' }, responseType: 'blob', timeout: 15000, donNotShowLoading: true })
  const blob = response?.data
  // JSON failures returned with HTTP 200 must never become an image URL.
  if (!(blob instanceof Blob) || !['image/jpeg', 'image/png', 'image/webp'].includes(blob.type) || blob.size === 0 || blob.size > 16 * 1024 * 1024) throw new Error('preview_unavailable')
  return blob
}
