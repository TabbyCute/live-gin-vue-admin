import { useRoute } from 'vue-router'
import { useUserStore } from '@/pinia/modules/user'
import { isSuperAdmin } from '@/utils/superAdmin'

export const useBtnAuth = () => {
  const route = useRoute()
  const userStore = useUserStore()
  return new Proxy({}, {
    get: (_, name) => {
      // Vue probes these properties when unwrapping setup bindings.
      if (typeof name !== 'string' || name.startsWith('__v_')) return undefined
      return isSuperAdmin(userStore.userInfo) || route.meta.btns?.[name]
    }
  })
}
