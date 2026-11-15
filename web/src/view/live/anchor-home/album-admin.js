import { computed, reactive, ref } from 'vue'

export const photoStatuses = { pending: '待审核', approved: '已通过', rejected: '已驳回', removed: '已下架', deleted: '已删除' }
const messages = {
  version_conflict: '照片状态已被其他操作更新，请重新打开后审核。',
  idempotency_conflict: '请求编号与原操作不一致，请刷新后核实。',
  forbidden: '当前账号无权操作主播相册。',
  not_found: '照片已不存在，请刷新列表。',
  invalid: '审核参数不正确，请检查原因和照片状态。',
  busy: '服务繁忙，可使用原请求重试。',
  unavailable: '服务暂时不可用，可使用原请求重试。'
}

/** Separate view state from transport so stale replies and retries are testable. */
export function useAlbumAdmin({ list, preview, review, newRequestId, createURL, revokeURL, success }) {
  const query = reactive({ page: 1, pageSize: 20, status: 'pending', anchorNo: '' })
  const rows = ref([]), total = ref(0), loading = ref(false), error = ref('')
  const visible = ref(false), selected = ref(null), imageURL = ref(''), previewLoading = ref(false)
  const decision = ref('approve'), reason = ref(''), saving = ref(false), pending = ref(null)
  let epoch = 0, listSeq = 0, previewSeq = 0
  const reasonValid = computed(() => {
    const value = reason.value.trim()
    return (decision.value === 'approve' || value.length > 0) && [...value].length <= 200 && !/[\u0000-\u001f\u007f-\u009f]/u.test(value)
  })
  const clearImage = () => { if (imageURL.value) revokeURL(imageURL.value); imageURL.value = '' }
  function closePreview() { previewSeq++; clearImage(); previewLoading.value = false; visible.value = false; selected.value = null }
  function clear() {
    epoch++; listSeq++; closePreview(); rows.value = []; total.value = 0; loading.value = false
    saving.value = false; pending.value = null; error.value = ''; reason.value = ''; decision.value = 'approve'
  }
  async function load() {
    const generation = epoch, seq = ++listSeq
    loading.value = true; error.value = ''
    try {
      const res = await list({ ...query, anchorNo: query.anchorNo.trim() })
      if (generation !== epoch || seq !== listSeq) return
      if (res?.code !== 0) throw res
      rows.value = res.data.list || []; total.value = Number(res.data.total) || 0
    } catch { if (generation === epoch && seq === listSeq) { rows.value = []; total.value = 0; error.value = '列表加载失败，请核实权限后重试。' } }
    finally { if (generation === epoch && seq === listSeq) loading.value = false }
  }
  function search() { query.page = 1; return load() }
  async function open(row) {
    if (saving.value || pending.value) return
    closePreview(); selected.value = { ...row }; visible.value = true; reason.value = ''
    decision.value = row.status === 'approved' ? 'takedown' : 'approve'
    const generation = epoch, seq = ++previewSeq
    previewLoading.value = true; error.value = ''
    try {
      const blob = await preview(row.photoNo)
      if (generation !== epoch || seq !== previewSeq || !visible.value) return
      imageURL.value = createURL(blob)
    } catch { if (generation === epoch && seq === previewSeq) error.value = '图片预览失败，请核实权限后重新打开。' }
    finally { if (generation === epoch && seq === previewSeq) previewLoading.value = false }
  }
  async function submit() {
    if (saving.value || !selected.value || !imageURL.value || !reasonValid.value || pending.value) return
    if (!(selected.value.status === 'pending' && ['approve', 'reject'].includes(decision.value)) && !(selected.value.status === 'approved' && decision.value === 'takedown')) return
    pending.value = Object.freeze({ requestId: newRequestId(), photoNo: selected.value.photoNo, version: selected.value.version, decision: decision.value, reason: reason.value.trim() })
    return retryPending()
  }
  async function retryPending() {
    if (!pending.value || saving.value) return
    const generation = epoch, command = pending.value
    saving.value = true; error.value = ''
    try {
      const res = await review({ ...command })
      if (generation !== epoch) return
      if (res?.code !== 0) {
        const code = res?.data?.errorCode
        error.value = messages[code] || '操作结果尚未确认，请使用原请求重试。'
        // A definitive rejection allows a fresh version to be loaded. Transient
        // failures may follow a committed transaction, so retain its request ID.
        if (['invalid', 'forbidden', 'not_found', 'version_conflict', 'idempotency_conflict', 'quota'].includes(code)) {
          pending.value = null; closePreview()
          const message = error.value; await load(); if (generation === epoch) error.value = message
        }
        return
      }
      pending.value = null; closePreview(); success('审核操作已完成'); await load()
    } catch { if (generation === epoch) error.value = '响应中断，操作结果未知；请使用原请求重试以核实结果。' }
    finally { if (generation === epoch) saving.value = false }
  }
  return { query, rows, total, loading, error, visible, selected, imageURL, previewLoading, decision, reason, reasonValid, saving, pending, load, search, open, submit, retryPending, closePreview, clear }
}
