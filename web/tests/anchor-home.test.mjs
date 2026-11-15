import test from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import vm from 'node:vm'
import { parse, compileScript, compileTemplate } from '@vue/compiler-sfc'
import * as vue from 'vue'
import { useAlbumAdmin, photoStatuses } from '../src/view/live/anchor-home/album-admin.js'

const deferred = () => { let resolve, reject; const promise = new Promise((a, b) => { resolve = a; reject = b }); return { promise, resolve, reject } }
const row = { photoNo: 'photo_1', anchorNo: 'anchor_1', version: 1, status: 'pending' }
const response = (photoNo) => ({ code: 0, data: { list: [{ photoNo }], total: 1 } })
function admin(overrides = {}) {
  const revoked = [], created = []; let serial = 0
  const state = useAlbumAdmin({
    list: async () => response('photo_1'), preview: async () => new Blob(['image'], { type: 'image/jpeg' }), review: async () => ({ code: 0 }),
    newRequestId: () => `request_${++serial}`, createURL: blob => { created.push(blob); return `blob:${serial++}` }, revokeURL: url => revoked.push(url), success() {}, ...overrides
  })
  return { state, revoked, created }
}

test('admin template and script compile', async () => {
  const source = await readFile(new URL('../src/view/live/anchor-home/index.vue', import.meta.url), 'utf8')
  const { descriptor } = parse(source)
  assert.deepEqual(compileTemplate({ source: descriptor.template.content, filename: 'index.vue', id: 'album-test' }).errors, [])
  assert.ok(compileScript(descriptor, { id: 'album-test' }).content)
})

async function component() {
  const source = await readFile(new URL('../src/view/live/anchor-home/index.vue', import.meta.url), 'utf8')
  const { descriptor } = parse(source), { content } = compileScript(descriptor, { id: 'album-view-test' })
  const mounted = [], activated = [], deactivated = [], cleanup = [], revoked = []
  const user = vue.reactive({ token: 'session', userInfo: { ID: 1, authorityId: 888 } }); let lists = 0
  const dependencies = {
    vue: { ...vue, onMounted: fn => mounted.push(fn), onActivated: fn => activated.push(fn), onDeactivated: fn => deactivated.push(fn), onBeforeUnmount: fn => cleanup.push(fn) },
    'element-plus': { ElMessage: { success() {} } }, '@/pinia/modules/user': { useUserStore: () => user },
    '@/utils/format': { CreateUUID: () => 'component-request' }, '@/utils/date': { formatTimeToStr: String },
    './album-admin': { useAlbumAdmin, photoStatuses },
    '@/api/anchor-home': { listAnchorPhotos: async () => { lists++; return response('fixture') }, previewAnchorPhoto: async () => new Blob(['image']), reviewAnchorPhoto: async () => ({ code: 0 }) }
  }
  const context = vm.createContext({ URL: { createObjectURL: () => 'blob:private', revokeObjectURL: value => revoked.push(value) } })
  const module = new vm.SourceTextModule(content, { context })
  await module.link(name => { const dependency = dependencies[name]; assert.ok(dependency, name); return new vm.SyntheticModule(Object.keys(dependency), function () { for (const [key, value] of Object.entries(dependency)) this.setExport(key, value) }, { context }) })
  await module.evaluate()
  const scope = vue.effectScope(), state = scope.run(() => module.namespace.default.setup({}, { expose() {} }))
  return { state, user, mounted, activated, deactivated, revoked, lists: () => lists, close() { cleanup.forEach(fn => fn()); scope.stop() } }
}

test('component immediately clears private preview when account or role changes', async t => {
  const p = await component(); t.after(p.close)
  for (const change of [() => { p.user.userInfo.ID = 2 }, () => { p.user.userInfo.authorityId = 123 }, () => { p.user.token = '' }]) {
    await p.state.open(row); assert.equal(p.state.imageURL.value, 'blob:private'); change()
    assert.equal(p.state.imageURL.value, ''); assert.equal(p.state.selected.value, null)
  }
  assert.equal(p.revoked.length, 3)
})

test('ordinary mount and keepalive activation load once then clear and reload on return', async t => {
  const p = await component(); t.after(p.close)
  p.mounted.forEach(fn => fn()); p.activated.forEach(fn => fn()); await Promise.resolve()
  assert.equal(p.lists(), 1)
  await p.state.open(row); p.deactivated.forEach(fn => fn()); assert.equal(p.state.imageURL.value, '')
  p.activated.forEach(fn => fn()); await Promise.resolve(); assert.equal(p.lists(), 2)
})

test('late list results cannot replace new filters or previous-session data', async () => {
  const a = deferred(), b = deferred(), c = deferred(); let calls = 0
  const { state: s } = admin({ list: () => [a, b, c][calls++].promise })
  const first = s.load(), second = s.load(); b.resolve(response('new')); await second; a.resolve(response('old')); await first
  assert.equal(s.rows.value[0].photoNo, 'new')
  const third = s.load(); s.clear(); c.resolve(response('private')); await third
  assert.deepEqual(s.rows.value, []); assert.equal(s.total.value, 0)
})

test('late previews never recreate revoked private media after close or identity switch', async () => {
  const pending = deferred(), { state: s, created, revoked } = admin({ preview: () => pending.promise })
  const work = s.open(row); s.closePreview(); pending.resolve(new Blob(['private'])); await work
  assert.equal(created.length, 0); assert.equal(s.imageURL.value, '')
  await s.open(row); assert.equal(created.length, 1); s.clear()
  assert.equal(revoked.length, 1); assert.equal(s.selected.value, null); assert.equal(s.imageURL.value, '')
})

test('switching photos revokes the previous Blob URL', async () => {
  const { state: s, revoked } = admin()
  await s.open(row); const first = s.imageURL.value
  await s.open({ ...row, photoNo: 'photo_2' }); assert.deepEqual(revoked, [first])
  s.closePreview(); assert.equal(revoked.length, 2)
})

test('unknown outcome keeps immutable request and blocks another moderation target', async () => {
  const calls = []; const { state: s } = admin({ review: async body => { calls.push(body); if (calls.length === 1) throw new Error('connection_lost'); return { code: 0 } } })
  await s.open(row); s.decision.value = 'reject'; s.reason.value = '不适合公开展示'; await s.submit()
  assert.ok(s.pending.value); await s.open({ ...row, photoNo: 'photo_2' }); assert.equal(s.selected.value.photoNo, row.photoNo)
  s.reason.value = 'changed'; s.decision.value = 'approve'; await s.retryPending()
  assert.deepEqual(calls[0], calls[1]); assert.equal(s.pending.value, null)
})

test('transient business failure preserves original command, known CAS conflict clears stale photo', async () => {
  for (const code of ['busy', 'unavailable', 'version_conflict', 'forbidden']) {
    const { state: s } = admin({ review: async () => ({ code: 7, data: { errorCode: code } }) })
    await s.open(row); await s.submit()
    if (['busy', 'unavailable'].includes(code)) assert.ok(s.pending.value)
    else { assert.equal(s.pending.value, null); assert.equal(s.visible.value, false); assert.equal(s.imageURL.value, '') }
  }
})

test('reason boundaries and state transitions reject accidental commands', async () => {
  let calls = 0; const { state: s } = admin({ review: async () => { calls++; return { code: 0 } } })
  await s.open(row); s.decision.value = 'reject'
  for (const reason of ['', ' ', 'a\nb', '中'.repeat(201)]) { s.reason.value = reason; await s.submit(); assert.equal(calls, 0) }
  s.reason.value = '😀'.repeat(200); assert.equal(s.reasonValid.value, true)
  s.decision.value = 'takedown'; await s.submit(); assert.equal(calls, 0)
  s.decision.value = 'reject'; await s.submit(); assert.equal(calls, 1)
  await s.open({ ...row, status: 'rejected' }); await s.submit(); assert.equal(calls, 1)
})

test('double clicks dispatch once and old-session review cannot reload private data', async () => {
  const result = deferred(); let calls = 0, lists = 0
  const { state: s } = admin({ review: () => { calls++; return result.promise }, list: async () => { lists++; return response('secret') } })
  await s.open(row); const work = s.submit(); await s.submit(); assert.equal(calls, 1)
  s.clear(); result.resolve({ code: 0 }); await work
  assert.equal(lists, 0); assert.deepEqual(s.rows.value, []); assert.equal(s.pending.value, null)
})

test('preview API uses authenticated fixed route and rejects JSON, SVG and oversized payloads', async () => {
  const source = await readFile(new URL('../src/api/anchor-home.js', import.meta.url), 'utf8')
  const calls = []; let blob = new Blob(['image'], { type: 'image/jpeg' })
  const context = vm.createContext({ Blob, Error })
  const mod = new vm.SourceTextModule(source, { context })
  await mod.link(() => new vm.SyntheticModule(['default'], function () { this.setExport('default', async config => { calls.push(config); return { data: blob } }) }, { context }))
  await mod.evaluate(); await mod.namespace.previewAnchorPhoto('photo_1')
  assert.equal(calls[0].url, '/v1/admin/anchor-home/album/preview'); assert.equal(calls[0].responseType, 'blob')
  assert.equal(calls[0].params.photoNo, 'photo_1'); assert.equal(calls[0].params.variant, 'full'); assert.equal(calls[0].params.token, undefined)
  for (const invalid of [new Blob(['{"code":7}'], { type: 'application/json' }), new Blob(['<svg/>'], { type: 'image/svg+xml' }), new Blob([], { type: 'image/png' }), new Blob([new Uint8Array(16 * 1024 * 1024 + 1)], { type: 'image/jpeg' })]) {
    blob = invalid; await assert.rejects(mod.namespace.previewAnchorPhoto('photo_1'))
  }
})
