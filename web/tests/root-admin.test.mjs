import test from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import vm from 'node:vm'
import { isRef, proxyRefs } from 'vue'
import { isSuperAdmin } from '../src/utils/superAdmin.js'

async function load(path, mocks) {
  const context = vm.createContext({})
  const module = new vm.SourceTextModule(await readFile(new URL(path, import.meta.url), 'utf8'), { context })
  await module.link(specifier => {
    const values = mocks[specifier]
    assert.ok(values, specifier)
    return new vm.SyntheticModule(Object.keys(values), function () {
      for (const [key, value] of Object.entries(values)) this.setExport(key, value)
    }, { context })
  })
  await module.evaluate()
  return module.namespace
}

test('root sees unassigned buttons and role directives; other accounts keep their permissions', async () => {
  const user = { userInfo: { ID: 1, authorityId: 0 } }
  const route = { meta: { btns: { read: 999 } } }
  const mocks = {
    '@/utils/superAdmin': { isSuperAdmin },
    '@/pinia/modules/user': { useUserStore: () => user },
    'vue-router': { useRoute: () => route }
  }
  const { useBtnAuth } = await load('../src/utils/btnAuth.js', mocks)
  const auth = useBtnAuth()
  assert.equal(auth.unassigned, true)
  assert.equal(isRef(auth), false)
  assert.equal(proxyRefs({ auth }).auth.unassigned, true)
  let directive
  const plugin = await load('../src/directive/auth.js', mocks)
  plugin.default.install({ directive: (_, value) => { directive = value } })
  let removed = false
  const el = { parentNode: { removeChild() { removed = true } } }
  directive.mounted(el, { value: 888, modifiers: {} })
  assert.equal(removed, false)
  user.userInfo = { ID: '1', authorityId: 999 }
  assert.equal(auth.unassigned, true)
  user.userInfo = { ID: 2, authorityId: 999 }
  assert.equal(auth.unassigned, undefined)
  assert.equal(auth.read, 999)
  directive.mounted(el, { value: 888, modifiers: {} })
  assert.equal(removed, true)
  route.meta.btns = { write: 999 }
  assert.equal(auth.read, undefined)
  assert.equal(auth.write, 999)
  assert.equal(isSuperAdmin({ ID: 2, authorityId: 1 }), false)
  assert.equal(isSuperAdmin({ id: 1 }), false)
})
