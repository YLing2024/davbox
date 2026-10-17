// 窄屏 / 触摸响应式的静态断言。
//
// jsdom 量不到真实布局尺寸，这里按需求书允许的降级口径断言「class/style/CSS 规则」：
// 真实尺寸（390×844）由自证报告里的 Chrome CDP 实测数据佐证。
// 断点必须只命中窄屏或 pointer: coarse，桌面（≥992px 且指针精确）保持原样。
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

const here = dirname(fileURLToPath(import.meta.url))
const srcDir = join(here, '..', 'src')

function readSrc(rel: string): string {
  return readFileSync(join(srcDir, rel), 'utf8')
}

function walkFiles(dir: string, exts: string[]): string[] {
  const out: string[] = []
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) out.push(...walkFiles(full, exts))
    else if (exts.some((e) => full.endsWith(e))) out.push(full)
  }
  return out
}

// 截取目标媒体查询到下一个顶层 `@media`（或文件尾）之间的一段，避免跨断点误判。
function mediaBlock(css: string, header: string): string {
  const start = css.indexOf(header)
  assert.notEqual(start, -1, `styles.css 应包含媒体查询 ${header}`)
  const next = css.indexOf('@media', start + header.length)
  return css.slice(start, next === -1 ? undefined : next)
}

test('触摸断点只落在窄屏或 pointer: coarse，且不碰 ≥992px 精确指针桌面', () => {
  const css = readSrc('styles.css')
  const touch = mediaBlock(css, '@media (max-width: 991px), (pointer: coarse)')
  assert.match(touch, /max-width:\s*991px/)
  assert.match(touch, /pointer:\s*coarse/)
  // 不允许出现会把桌面一起命中的宽断点
  assert.doesNotMatch(css, /@media\s*\(min-width:\s*992px\)[\s\S]{0,80}(min-height:\s*44px|font-size:\s*16px)/)
})

test('主要操作 ≥44px、次要图标 ≥40×40', () => {
  const touch = mediaBlock(readSrc('styles.css'), '@media (max-width: 991px), (pointer: coarse)')
  assert.match(touch, /\.ant-btn:not\(\.ant-btn-icon-only\)\s*\{[^}]*min-height:\s*44px/)
  assert.match(touch, /\.ant-btn\.ant-btn-icon-only\s*\{[^}]*min-width:\s*40px/)
  assert.match(touch, /\.ant-btn\.ant-btn-icon-only\s*\{[^}]*min-height:\s*40px/)
})

test('表单控件 ≥44px 且字号 ≥16px（含 antd Select 内层 .ant-select-selector）', () => {
  const touch = mediaBlock(readSrc('styles.css'), '@media (max-width: 991px), (pointer: coarse)')
  assert.match(touch, /\.ant-input-affix-wrapper[^{]*\{[^}]*min-height:\s*44px/)
  assert.match(touch, /\.ant-select\s+\.ant-select-selector/)
  assert.match(touch, /min-height:\s*44px/)
  assert.match(touch, /font-size:\s*16px\s*!important/)
})

test('行内相邻操作间距 ≥8px 且工具栏可换行', () => {
  const touch = mediaBlock(readSrc('styles.css'), '@media (max-width: 991px), (pointer: coarse)')
  assert.match(touch, /\.davbox-actions\s*\{[^}]*column-gap:\s*8px/)
  assert.match(touch, /\.davbox-toolbar\s*\{[^}]*flex-wrap:\s*wrap/)
})

test('antd 行内 code/kbd 的 0.85em（≈11.9px）在触摸断点抬到 12px', () => {
  const touch = mediaBlock(readSrc('styles.css'), '@media (max-width: 991px), (pointer: coarse)')
  assert.match(touch, /\.ant-typography\s+code,[\s\S]{0,80}\.ant-typography\s+kbd\s*\{[^}]*font-size:\s*12px/)
})

test('Switch 命中区放大到 ≥44px 且不改可见尺寸', () => {
  const touch = mediaBlock(readSrc('styles.css'), '@media (max-width: 991px), (pointer: coarse)')
  assert.match(touch, /\.ant-switch::after\s*\{[^}]*inset:\s*-14px\s+-8px/)
})

test('长账号名/路径在窄屏省略，且窄屏顶栏账号名同样省略', () => {
  const css = readSrc('styles.css')
  const narrow = mediaBlock(css, '@media (max-width: 991px) {')
  assert.match(narrow, /\.davbox-ellipsis\s*\{[^}]*text-overflow:\s*ellipsis/)
  assert.match(narrow, /\.davbox-who\s*\{[^}]*text-overflow:\s*ellipsis/)
})

test('页面把响应式 class 挂到实际的工具栏/操作列/长文本上', () => {
  const admin = readSrc('admin/AdminPage.tsx')
  assert.match(admin, /className="davbox-actions"/)
  assert.match(admin, /className="davbox-toolbar"/)
  assert.match(admin, /className="davbox-ellipsis"/)
  const client = readSrc('client/ClientPage.tsx')
  assert.match(client, /className="davbox-actions"/)
  assert.match(client, /className="davbox-toolbar"/)
  assert.match(client, /className="davbox-ellipsis"/)
  assert.match(readSrc('appChrome.tsx'), /className="davbox-who"/)
})

test('全仓可见字号声明均 ≥12px（px / rem / em / 百分比）', () => {
  const problems: string[] = []
  for (const file of walkFiles(srcDir, ['.css', '.tsx', '.ts'])) {
    const text = readFileSync(file, 'utf8')
    const patterns: { re: RegExp; toPx: (v: number, unit: string) => number }[] = [
      { re: /font-size:\s*([0-9.]+)(px|rem|em|%)?/g, toPx: (v, u) => (u === 'rem' || u === 'em' ? v * 16 : u === '%' ? (v / 100) * 16 : v) },
      { re: /fontSize:\s*([0-9.]+)/g, toPx: (v) => v },
    ]
    for (const { re, toPx } of patterns) {
      for (const m of text.matchAll(re)) {
        const value = Number(m[1])
        const unit = m[2] ?? 'px'
        const px = toPx(value, unit)
        if (px < 12) problems.push(`${file}: ${m[0].trim()} → ${px}px`)
      }
    }
  }
  assert.deepEqual(problems, [], `存在 <12px 的字号声明:\n${problems.join('\n')}`)
})
