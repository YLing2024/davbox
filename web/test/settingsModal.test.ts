import { test } from 'node:test'
import assert from 'node:assert/strict'

import { initialSettingsModalState, settingsModalReducer } from '../src/settingsModal.ts'

// --- 「设置」弹窗（跨域白名单）的状态流转 ---

const SAVED = ['https://app.example.com', 'http://127.0.0.1:18900']

test('初始不显示设置弹窗', () => {
  const s = initialSettingsModalState()
  assert.equal(s.open, false)
})

test('点击设置打开弹窗并显示当前生效值', () => {
  const s = settingsModalReducer(initialSettingsModalState(), { type: 'open', origins: SAVED })
  assert.equal(s.open, true)
  assert.equal(s.text, SAVED.join('\n'))
  assert.equal(s.error, '')
  assert.equal(s.saved, false)
})

test('编辑会清掉上一次的错误与成功提示', () => {
  let s = settingsModalReducer(initialSettingsModalState(), { type: 'open', origins: SAVED })
  s = settingsModalReducer(s, { type: 'save-error', message: '第 1 行不合法' })
  s = settingsModalReducer(s, { type: 'edit', text: 'https://x.example.com' })
  assert.equal(s.text, 'https://x.example.com')
  assert.equal(s.error, '')
  assert.equal(s.saved, false)
})

test('保存成功后回填规整值并提示，保持打开', () => {
  let s = settingsModalReducer(initialSettingsModalState(), { type: 'open', origins: SAVED })
  s = settingsModalReducer(s, { type: 'edit', text: 'https://b.example.com' })
  s = settingsModalReducer(s, { type: 'save-start' })
  assert.equal(s.saving, true)
  s = settingsModalReducer(s, { type: 'save-ok', origins: ['https://b.example.com'] })
  assert.equal(s.open, true)
  assert.equal(s.saved, true)
  assert.equal(s.saving, false)
  assert.equal(s.text, 'https://b.example.com')
})

test('保存失败在弹窗内报错并保持打开', () => {
  let s = settingsModalReducer(initialSettingsModalState(), { type: 'open', origins: SAVED })
  s = settingsModalReducer(s, {
    type: 'save-error',
    message: '第 2 行不合法：「not-a-url」。应为 scheme://host 或 scheme://host:port，scheme 限 http/https',
  })
  assert.equal(s.open, true)
  assert.equal(s.saved, false)
  assert.equal(s.saving, false)
  assert.match(s.error, /第 2 行不合法/)
})

test('关闭丢弃未保存编辑，重新打开显示已保存值', () => {
  let s = settingsModalReducer(initialSettingsModalState(), { type: 'open', origins: SAVED })
  s = settingsModalReducer(s, { type: 'edit', text: 'https://draft.example.com' })
  s = settingsModalReducer(s, { type: 'close' })
  assert.equal(s.open, false)
  s = settingsModalReducer(s, { type: 'open', origins: SAVED })
  assert.equal(s.text, SAVED.join('\n'))
})

test('保存关闭后再打开显示新值', () => {
  let s = settingsModalReducer(initialSettingsModalState(), { type: 'open', origins: SAVED })
  s = settingsModalReducer(s, { type: 'save-ok', origins: ['https://new.example.com'] })
  s = settingsModalReducer(s, { type: 'close' })
  s = settingsModalReducer(s, { type: 'open', origins: ['https://new.example.com'] })
  assert.equal(s.text, 'https://new.example.com')
})
