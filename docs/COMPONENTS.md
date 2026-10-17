# 可复用组件登记表

管理端（`web/`）一律使用 Ant Design 5 默认设计语言（主色 `#1677ff`、圆角 6px、系统字体栈）。
组件不新造颜色/圆角/字号，配色与尺寸走 antd 默认；`ConfigProvider.theme` 只切 `algorithm`。

| 组件 | 用途 | 关键 props / 事件 | 最小示例 | 注意事项 |
|---|---|---|---|---|
| `AppHeader` | 管理端 / 文件页共用的吸顶顶栏：品牌、当前账号、主题切换与页面操作 | `title: string`、`tag: string`、`who?: string`（窄屏超长自动省略）、`actions?: ReactNode` | `<AppHeader title="davbox" tag="管理" actions={<Button>设置</Button>} />` | 窄屏（<768px）把主题切换与 `actions` 收进右侧抽屉，不隐藏入口；窄屏下 `who` 用 `davbox-who` 省略号，`title` 提供全文 |
| `ThemeToggle` | 三态主题切换（自动 / 亮色 / 暗色） | `block?: boolean`（默认 `false`，抽屉/窄屏传 `true`） | `<ThemeToggle block />` | 状态由 `theme.ts` 的 `useThemeMode()` 管理，持久化键 `localStorage.theme_mode`；`auto` 实时跟随系统，不得在此组件内自行读写存储 |
