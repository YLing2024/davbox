// 「设置」弹窗（跨域白名单）的状态机，独立于 React 以便直接测试。
// 弹窗打开时以当前生效值填充文本域；关闭即丢弃未保存的编辑。

export interface SettingsModalState {
  open: boolean
  text: string
  error: string
  saved: boolean
  saving: boolean
}

export type SettingsModalAction =
  | { type: 'open'; origins: string[] }
  | { type: 'close' }
  | { type: 'edit'; text: string }
  | { type: 'save-start' }
  | { type: 'save-error'; message: string }
  | { type: 'save-ok'; origins: string[] }

export function initialSettingsModalState(): SettingsModalState {
  return { open: false, text: '', error: '', saved: false, saving: false }
}

export function settingsModalReducer(
  state: SettingsModalState,
  action: SettingsModalAction,
): SettingsModalState {
  switch (action.type) {
    case 'open':
      // 以当前生效值打开，丢弃上一次的编辑与提示。
      return {
        open: true,
        text: action.origins.join('\n'),
        error: '',
        saved: false,
        saving: false,
      }
    case 'close':
      // 丢弃未保存的编辑，重新打开时显示已保存值。
      return initialSettingsModalState()
    case 'edit':
      // 一有编辑就清掉上一次的错误与成功提示。
      return { ...state, text: action.text, error: '', saved: false }
    case 'save-start':
      return { ...state, saving: true, error: '', saved: false }
    case 'save-error':
      // 在弹窗内报错，保持打开。
      return { ...state, saving: false, error: action.message, saved: false }
    case 'save-ok':
      // 回填服务端规整后的值并提示，保持打开。
      return {
        open: true,
        text: action.origins.join('\n'),
        error: '',
        saved: true,
        saving: false,
      }
    default:
      return state
  }
}
