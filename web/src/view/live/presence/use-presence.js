import { computed, onActivated, onBeforeUnmount, onDeactivated, ref, watch } from 'vue'
import { getLivePresenceStats } from '@/api/live-presence'
import { useUserStore } from '@/pinia/modules/user'
import { isSuperAdmin } from '@/utils/superAdmin'

export const presenceCount = (value, field) => {
  if (!value || value.stale || value[field] === '') return '—'
  return value[field] ?? '—'
}

/** One bounded batch per page, no overlapping timers or late page overwrite. */
export function usePresence(rows, field = 'sessionNo') {
  const user = useUserStore()
  const allowed = computed(() => isSuperAdmin(user.userInfo) || Number(user.userInfo?.authorityId) === 888)
  const stats = ref({})
  let timer, generation = 0, active = true
  const stop = () => { clearTimeout(timer); timer = undefined; generation++ }
  const refresh = async () => {
    stop()
    if (!active || !allowed.value) return
    if (document.visibilityState === 'hidden') { timer = setTimeout(refresh, 10000); return }
    const version = generation
    const values = [...new Set(rows.value.map((row) => row[field]).filter(Boolean))].slice(0, 100)
    if (!values.length) { stats.value = {}; return }
    try {
      const response = await getLivePresenceStats({ [field === 'roomNo' ? 'roomNos' : 'sessionNos']: values })
      if (version !== generation) return
      stats.value = response.code === 0
        ? Object.fromEntries((response.data || []).map((row) => [row[field], row])) : {}
    } catch {
      if (version === generation) stats.value = {}
    } finally {
      if (version === generation && active) timer = setTimeout(refresh, 10000)
    }
  }
  watch(() => [allowed.value, rows.value.map((row) => row[field]).join(',')], refresh, { immediate: true })
  onActivated(() => { active = true; refresh() })
  onDeactivated(() => { active = false; stop() })
  onBeforeUnmount(() => { active = false; stop() })
  return { presenceStats: stats, presenceAllowed: allowed, refreshPresence: refresh }
}
