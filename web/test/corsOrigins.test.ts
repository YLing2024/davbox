import { test } from 'node:test'
import assert from 'node:assert/strict'

import { normalizeOrigin, parseCorsOrigins } from '../src/corsOrigins.ts'
import { adminApi } from '../src/api.ts'

// --- 来源解析与校验（设置弹窗渲染当前值、非法输入分支） ---

test('留空表示关闭跨域', () => {
  assert.deepEqual(parseCorsOrigins(''), { origins: [], error: null })
  assert.deepEqual(parseCorsOrigins('  \n , \n'), { origins: [], error: null })
})

test('当前值一行一个展示并可原样解析回来', () => {
  const current = ['https://app.example.com', 'http://127.0.0.1:18900']
  const rendered = current.join('\n') // 设置弹窗文本域的展示形式
  assert.equal(rendered, 'https://app.example.com\nhttp://127.0.0.1:18900')
  assert.deepEqual(parseCorsOrigins(rendered), { origins: current, error: null })
})

test('支持逗号分隔、大小写规整与去重', () => {
  const parsed = parseCorsOrigins('HTTPS://App.Example.com/, https://b.example.com:8443\nhttp://app.example.com')
  assert.equal(parsed.error, null)
  assert.deepEqual(parsed.origins, [
    'https://app.example.com',
    'https://b.example.com:8443',
    'http://app.example.com',
  ])
})

test('非法项当场报错并指出行号，不静默丢弃', () => {
  const parsed = parseCorsOrigins('https://ok.example.com\nftp://bad.example.com')
  assert.equal(parsed.origins.length, 0)
  assert.ok(parsed.error)
  assert.match(parsed.error, /第 2 行/)
  assert.match(parsed.error, /ftp:\/\/bad\.example\.com/)
})

test('非法项错误文案与服务端口径一致并指出行号', () => {
  const parsed = parseCorsOrigins('https://ok.example.com\nnot-a-url')
  assert.deepEqual(parsed.origins, [])
  assert.equal(
    parsed.error,
    '第 2 行不合法：「not-a-url」。应为 scheme://host 或 scheme://host:port，scheme 限 http/https',
  )
})

test('逗号分隔中的非法项也定位到所在行', () => {
  const parsed = parseCorsOrigins('https://a.example.com, not-a-url')
  assert.ok(parsed.error)
  assert.match(parsed.error, /第 1 行/)
})

test('normalizeOrigin 拒绝常见非法形式', () => {
  const bad = [
    'ftp://a.example.com',
    'a.example.com',
    'https://',
    'https://a.example.com/foo',
    'https://a.example.com?x=1',
    'https://user@a.example.com',
    '*',
    'https://a.example.com:abc',
  ]
  for (const raw of bad) {
    assert.equal(normalizeOrigin(raw), null, `应拒绝 ${raw}`)
  }
  assert.equal(normalizeOrigin('HTTPS://App.Example.com/'), 'https://app.example.com')
  assert.equal(normalizeOrigin('http://127.0.0.1:18900'), 'http://127.0.0.1:18900')
})

// --- 保存/加载调用的接口（保存按钮与加载当前值） ---

function withStubbedFetch(
  responder: (url: string, init: RequestInit | undefined) => Response,
  fn: (calls: { url: string; init?: RequestInit }[]) => Promise<void>,
): Promise<void> {
  const calls: { url: string; init?: RequestInit }[] = []
  const original = globalThis.fetch
  globalThis.fetch = (async (url: unknown, init?: RequestInit) => {
    const target = String(url)
    calls.push({ url: target, init })
    return responder(target, init)
  }) as typeof fetch
  return fn(calls).finally(() => {
    globalThis.fetch = original
  })
}

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })
}

test('保存调用 PUT /api/admin/settings 并回填规整值', async () => {
  await withStubbedFetch(
    () => jsonResponse({ corsOrigins: ['https://app.example.com'] }),
    async (calls) => {
      const saved = await adminApi.saveSettings(['HTTPS://App.Example.com/'])
      assert.deepEqual(saved.corsOrigins, ['https://app.example.com'])
      assert.equal(calls.length, 1)
      assert.equal(calls[0].url, '/api/admin/settings')
      assert.equal(calls[0].init?.method, 'PUT')
      assert.deepEqual(JSON.parse(String(calls[0].init?.body)), {
        corsOrigins: ['HTTPS://App.Example.com/'],
      })
    },
  )
})

test('加载调用 GET /api/admin/settings', async () => {
  await withStubbedFetch(
    () => jsonResponse({ corsOrigins: ['https://app.example.com'] }),
    async (calls) => {
      const got = await adminApi.getSettings()
      assert.deepEqual(got.corsOrigins, ['https://app.example.com'])
      assert.equal(calls[0].url, '/api/admin/settings')
      assert.equal(calls[0].init?.method, 'GET')
    },
  )
})

test('保存失败时抛出后端错误信息', async () => {
  await withStubbedFetch(
    () =>
      new Response(JSON.stringify({ error: '第 2 行不合法：「ftp://bad.example.com」' }), {
        status: 400,
        headers: { 'Content-Type': 'application/json' },
      }),
    async () => {
      await assert.rejects(
        () => adminApi.saveSettings(['https://ok.example.com', 'ftp://bad.example.com']),
        /第 2 行不合法/,
      )
    },
  )
})
