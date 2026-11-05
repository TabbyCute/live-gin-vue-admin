import test from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { createServer } from 'node:http'
import { once } from 'node:events'
import vm from 'node:vm'
import axios from 'axios'

test('钱包真实请求客户端保留鉴权、整数精度和失败重试参数', async (t) => {
  const calls = [], events = []
  const server = createServer(async (req, res) => {
    let body = ''; for await (const chunk of req) body += chunk
    calls.push({ url: req.url, token: req.headers['x-token'], body })
    res.setHeader('Content-Type', 'application/json')
    if (req.url.includes('busy=1')) {
      res.writeHead(503, { 'Retry-After': '1' }); res.end(JSON.stringify({ code: 47012, msg: '请使用原请求编号重试', data: {} })); return
    }
    res.end(JSON.stringify({ code: 0, data: { availableBalance: '9223372036854775807' } }))
  })
  server.listen(0, '127.0.0.1'); await once(server, 'listening')
  t.after(() => { server.closeAllConnections(); server.close() })
  const base = `http://127.0.0.1:${server.address().port}`
  const context = vm.createContext({ console, setTimeout, clearTimeout, document: { getElementById: () => null } })
  const store = { token: 'test-admin-token', userInfo: { ID: 1 }, setToken() {}, ClearStorage() {} }
  const stub = (exports) => new vm.SyntheticModule(Object.keys(exports), function () { for (const [k, v] of Object.entries(exports)) this.setExport(k, v) }, { context })
  const deps = {
    axios: stub({ default: axios }),
    '@/pinia/modules/user': stub({ useUserStore: () => store }),
    'element-plus': stub({ ElLoading: { service: () => ({ close() {} }) }, ElMessage: () => {} }),
    '@/utils/bus': stub({ emitter: { emit: (...args) => events.push(args) } }),
    '@/router/index': stub({ default: { push() {} } })
  }
  const client = new vm.SourceTextModule(await readFile(new URL('../src/utils/request.js', import.meta.url), 'utf8'), { context, initializeImportMeta: (meta) => { meta.env = { VITE_BASE_API: base } } })
  await client.link((specifier) => deps[specifier]); await client.evaluate()
  client.namespace.default.defaults.proxy = false
  const api = new vm.SourceTextModule(await readFile(new URL('../src/api/wallet.js', import.meta.url), 'utf8'), { context })
  await api.link(() => client); await api.evaluate()
  const result = await api.namespace.getWalletAccount({ phone: '+60123456789' })
  assert.equal(result.data.availableBalance, '9223372036854775807')
  assert.equal(calls[0].token, 'test-admin-token')
  assert.equal(new URL(calls[0].url, base).searchParams.get('phone'), '+60123456789')
  await assert.rejects(api.namespace.getWalletAccount({ busy: 1 }), (e) => e.response.status === 503 && e.response.data.code === 47012)
  assert.equal(events.at(-1)[1].code, 503)
  const request = { requestId: 'fixed_retry_001', accountNo: '10000010', amount: '9007199254740993', externalOrderNo: 'BANK-TEST' }
  await api.namespace.createWalletRecharge(request); await api.namespace.createWalletRecharge(request)
  assert.equal(calls.at(-1).body, calls.at(-2).body)
  assert.equal(JSON.parse(calls.at(-1).body).amount, request.amount)
  client.namespace.resetLoading()
})
