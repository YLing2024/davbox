// 三态主题（auto / light / dark）。
// - 持久化在 localStorage 的 theme_mode；
// - auto 时监听 window.matchMedia('(prefers-color-scheme: dark)') 的 change 事件实时跟随；
// - 解析结果写到 <html data-theme> 与 <meta name="color-scheme">，供首屏与自定义 CSS 使用。
// 读取失败 / 值非法 / localStorage 不可用一律回退 auto，不抛错、不白屏。
import { useSyncExternalStore } from 'react'

export type ThemeMode = 'auto' | 'light' | 'dark'
export type ResolvedTheme = 'light' | 'dark'

const STORAGE_KEY = 'theme_mode'
const DARK_QUERY = '(prefers-color-scheme: dark)'

export function isThemeMode(value: unknown): value is ThemeMode {
  return value === 'auto' || value === 'light' || value === 'dark'
}

export function getStoredThemeMode(): ThemeMode {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    return isThemeMode(raw) ? raw : 'auto'
  } catch {
    return 'auto'
  }
}

function prefersDark(): boolean {
  try {
    return window.matchMedia(DARK_QUERY).matches
  } catch {
    return false
  }
}

export function resolveTheme(mode: ThemeMode): ResolvedTheme {
  if (mode === 'dark') return 'dark'
  if (mode === 'light') return 'light'
  return prefersDark() ? 'dark' : 'light'
}

// 把解析后的明暗同步到 <html data-theme>、<html style.colorScheme> 与 color-scheme meta。
export function applyTheme(mode: ThemeMode): ResolvedTheme {
  const resolved = resolveTheme(mode)
  try {
    const root = document.documentElement
    root.setAttribute('data-theme', resolved)
    root.style.colorScheme = resolved
  } catch {
    /* 非浏览器环境忽略 */
  }
  try {
    const meta = document.querySelector('meta[name="color-scheme"]')
    if (meta) meta.setAttribute('content', resolved)
  } catch {
    /* 忽略 */
  }
  return resolved
}

export interface ThemeState {
  mode: ThemeMode
  resolved: ResolvedTheme
}

let state: ThemeState = (() => {
  const mode = getStoredThemeMode()
  return { mode, resolved: applyTheme(mode) }
})()

const listeners = new Set<() => void>()

function emit(): void {
  for (const listener of listeners) listener()
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

function getSnapshot(): ThemeState {
  return state
}

export function setThemeMode(mode: ThemeMode): void {
  try {
    window.localStorage.setItem(STORAGE_KEY, mode)
  } catch {
    /* localStorage 不可用时只在内存里生效 */
  }
  state = { mode, resolved: applyTheme(mode) }
  emit()
}

// auto 时跟随系统明暗变化，无需刷新。
try {
  const media = window.matchMedia(DARK_QUERY)
  media.addEventListener('change', () => {
    if (state.mode !== 'auto') return
    state = { mode: 'auto', resolved: applyTheme('auto') }
    emit()
  })
} catch {
  /* 不支持 matchMedia 时只保留手动切换 */
}

export interface UseThemeMode {
  mode: ThemeMode
  resolved: ResolvedTheme
  setMode: (mode: ThemeMode) => void
}

export function useThemeMode(): UseThemeMode {
  const snapshot = useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
  return { mode: snapshot.mode, resolved: snapshot.resolved, setMode: setThemeMode }
}
