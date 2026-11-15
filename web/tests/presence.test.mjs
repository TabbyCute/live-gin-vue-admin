import test from 'node:test'
import assert from 'node:assert/strict'
import vm from 'node:vm'
import { readFile } from 'node:fs/promises'
import * as vue from 'vue'

test('unknown/degraded counts are never displayed as zero; strings keep precision', async () => {
  const cleanups = [], timers = new Map(); let id = 0
  const pending = []
  const dependencies = {
    vue: { ...vue, onActivated() {}, onDeactivated: fn => cleanups.push(fn), onBeforeUnmount: fn => cleanups.push(fn) },
    '@/api/live-presence': { getLivePresenceStats: () => new Promise(resolve => pending.push(resolve)) },
    '@/pinia/modules/user': { useUserStore: () => ({ userInfo: { ID: 1, authorityId: 888 } }) },
    '@/utils/superAdmin': { isSuperAdmin: () => true }
  }
  const context = vm.createContext({ console, document: { visibilityState: 'visible' }, setTimeout: fn => { timers.set(++id, fn); return id }, clearTimeout: key => timers.delete(key) })
  const module = new vm.SourceTextModule(await readFile(new URL('../src/view/live/presence/use-presence.js', import.meta.url), 'utf8'), { context })
  await module.link(name => new vm.SyntheticModule(Object.keys(dependencies[name]), function () { for (const [key, value] of Object.entries(dependencies[name])) this.setExport(key, value) }, { context }))
  await module.evaluate()
  const { presenceCount, usePresence } = module.namespace
  assert.equal(presenceCount({ stale: true, guestCount: '99' }, 'guestCount'), '—')
  assert.equal(presenceCount({ guestCount: '9007199254740991' }, 'guestCount'), '9007199254740991')
  const scope = vue.effectScope(), rows = vue.ref([{ sessionNo: 'old' }]); const state = scope.run(() => usePresence(rows))
  rows.value = [{ sessionNo: 'new' }]; await vue.nextTick()
  pending[1]({ code: 0, data: [{ sessionNo: 'new', memberCount: '5' }] }); await Promise.resolve(); await Promise.resolve()
  pending[0]({ code: 0, data: [{ sessionNo: 'old', memberCount: '99' }] }); await Promise.resolve(); await Promise.resolve()
  assert.equal(state.presenceStats.value.new.memberCount, '5'); assert.equal(state.presenceStats.value.old, undefined); assert.equal(timers.size, 1)
  cleanups.forEach(fn => fn()); scope.stop(); assert.equal(timers.size, 0)
})
