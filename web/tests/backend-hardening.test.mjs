import test from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import vm from 'node:vm'
import { parse, compileScript, compileTemplate } from '@vue/compiler-sfc'
import * as vue from 'vue'

const deferred = () => { let resolve, reject; const promise = new Promise((a, b) => { resolve = a; reject = b }); return { promise, resolve, reject } }
async function page(file, overrides = {}) {
  const source = await readFile(new URL(`../src/view/${file}`, import.meta.url), 'utf8')
  const { descriptor } = parse(source)
  assert.deepEqual(compileTemplate({ source: descriptor.template.content, filename: file, id: 'hardening-test' }).errors, [])
  const { content } = compileScript(descriptor, { id: 'hardening-test' })
  const cleanup = [], user = vue.reactive({ token: 'admin', userInfo: { ID: 1, authorityId: 888 } }); let key = 0
  const list = async () => ({ code: 0, data: { list: [], total: '0' } })
  const deps = {
    vue: { ...vue, onMounted() {}, onBeforeUnmount(fn) { cleanup.push(fn) }, onDeactivated(fn) { cleanup.push(fn) } },
    'element-plus': { ElMessage: { success() {}, warning() {} }, ElMessageBox: {} },
    '@/pinia/modules/user': { useUserStore: () => user },
    '@/utils/format': { CreateUUID: () => `test-${++key}`, formatDate: String },
    '@/utils/date': { formatTimeToStr: String },
    '@/api/bank-reconciliation': { listBankBatches: list, getBankBatch: list, bankAction: list, ...overrides },
    '@/api/event-delivery': { listEventDeliveries: list, getEventDelivery: list, retryEventDelivery: list, ...overrides },
    '@/api/earning': { earningQuery: list, earningWrite: list, ...overrides },
    '../shared': { snapshot: value => JSON.parse(value || '{}') },
    '@/utils/request': { default: overrides.request || list }
  }
  const context = vm.createContext({ console, Error })
  const module = new vm.SourceTextModule(content, { context })
  await module.link(name => { const d = deps[name]; assert.ok(d, name); return new vm.SyntheticModule(Object.keys(d), function () { for (const [key, value] of Object.entries(d)) this.setExport(key, value) }, { context }) })
  await module.evaluate()
  const scope = vue.effectScope(), state = scope.run(() => module.namespace.default.setup({}, { expose() {} }))
  return { state, user, close() { cleanup.forEach(fn => fn()); scope.stop() } }
}

test('bank file selection ignores a late previous file and clears on logout', async t => {
  const old = deferred(), p = await page('payout/reconciliation/index.vue'); t.after(p.close)
  const read = p.state.selectFile({ target: { files: [{ size: 1, text: () => old.promise }] } })
  await p.state.selectFile({ target: { files: [{ size: 1, text: async () => 'NEW' }] } })
  old.resolve('OLD'); await read
  assert.equal(p.state.csv, 'NEW')
  p.user.token = ''; await vue.nextTick()
  assert.equal(p.state.csv, ''); assert.equal(p.state.csvReady.value, false)
})

for (const [file, api, method] of [
  ['operations/event-delivery/index.vue', 'listEventDeliveries', 'load'],
  ['live/earning/audit/index.vue', 'earningQuery', 'refresh'],
  ['payout/reconciliation/index.vue', 'listBankBatches', 'load']
]) test(`${file} drops stale and previous-account responses`, async t => {
  const old = deferred(), fresh = deferred(), late = deferred(); let count = 0
  const p = await page(file, { [api]: () => [old, fresh, late][count++].promise }); t.after(p.close)
  const a = p.state[method](), b = p.state[method]()
  fresh.resolve({ code: 0, data: { list: [{ id: 'new' }], total: '1' } }); await b
  old.resolve({ code: 0, data: { list: [{ id: 'old' }], total: '1' } }); await a
  assert.equal(p.state.rows.value[0].id, 'new')
  const work = p.state[method]()
  p.user.token = ''; await vue.nextTick()
  late.resolve({ code: 0, data: { list: [{ id: 'previous-user' }], total: '1' } }); await work
  assert.equal(p.state.rows.value.length, 0)
})

test('audit queue retries the original request after a lost response', async t => {
  const calls = [], p = await page('live/earning/audit/index.vue', { earningWrite: async (path, body) => { calls.push(JSON.stringify(body)); if (calls.length === 1) throw new Error('lost'); return { code: 0, data: {} } } }); t.after(p.close)
  p.state.anchorNo.value = 'anchor_1'; await p.state.start()
  p.state.anchorNo.value = 'anchor_2'; await p.state.submitPending()
  assert.equal(calls.length, 2); assert.equal(calls[0], calls[1])
})

test('operations token change cannot restore the previous session snapshot', async t => {
  const delayed = deferred(), p = await page('system/OperationsPanel.vue', { request: () => delayed.promise }); t.after(p.close)
  p.user.userInfo.ID = 2; p.user.token = ''; await vue.nextTick()
  delayed.resolve({ code: 0, data: { sensitiveOldSnapshot: true } }); await Promise.resolve(); await Promise.resolve()
  assert.equal(p.state.snapshot.value, null)
})
