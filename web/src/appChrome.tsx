// 管理端/文件页共用的顶栏：品牌、当前账号、主题切换与页面操作。
// 顶栏吸顶；窄屏（< 768px）把主题切换与操作收进抽屉，避免挤坏布局。
import { useEffect, useState, type ReactNode } from 'react'
import { Button, Drawer, Layout, Segmented, Space, Typography, theme } from 'antd'
import { IconMenu } from './icons'
import { useThemeMode, type ThemeMode } from './theme'

const { Header } = Layout
const { Text } = Typography

const THEME_OPTIONS: { label: string; value: ThemeMode }[] = [
  { label: '自动', value: 'auto' },
  { label: '亮色', value: 'light' },
  { label: '暗色', value: 'dark' },
]

export function ThemeToggle({ block = false }: { block?: boolean }) {
  const { mode, setMode } = useThemeMode()
  return (
    <Segmented
      block={block}
      value={mode}
      options={THEME_OPTIONS}
      onChange={(value) => setMode(value as ThemeMode)}
    />
  )
}

function useIsNarrow(query = '(max-width: 767px)'): boolean {
  const [narrow, setNarrow] = useState(() =>
    typeof window === 'undefined' ? false : window.matchMedia(query).matches,
  )
  useEffect(() => {
    const media = window.matchMedia(query)
    const update = () => setNarrow(media.matches)
    update()
    media.addEventListener('change', update)
    return () => media.removeEventListener('change', update)
  }, [query])
  return narrow
}

export function AppHeader({
  title,
  tag,
  who,
  actions,
}: {
  title: string
  tag: string
  who?: string
  actions?: ReactNode
}) {
  const narrow = useIsNarrow()
  const [open, setOpen] = useState(false)
  const { token } = theme.useToken()

  return (
    <Header
      style={{
        position: 'sticky',
        top: 0,
        zIndex: 20,
        display: 'flex',
        alignItems: 'center',
        gap: 10,
        lineHeight: 'normal',
        padding: '0 16px',
        background: token.colorBgContainer,
        borderBottom: `1px solid ${token.colorBorderSecondary}`,
      }}
    >
      <Text strong style={{ fontSize: 17 }}>
        {title}
      </Text>
      <Text type="secondary" style={{ fontSize: 12, letterSpacing: '0.08em' }}>
        {tag}
      </Text>
      {who ? (
        <Text type="secondary" style={{ fontSize: 13, marginLeft: 4 }}>
          {who}
        </Text>
      ) : null}
      <div style={{ flex: 1 }} />
      {narrow ? (
        <>
          <Button type="text" aria-label="菜单" icon={<IconMenu />} onClick={() => setOpen(true)} />
          <Drawer open={open} placement="right" width={280} onClose={() => setOpen(false)}>
            <Space direction="vertical" size="middle" style={{ width: '100%' }}>
              <ThemeToggle block />
              {actions}
            </Space>
          </Drawer>
        </>
      ) : (
        <Space size="middle">
          <ThemeToggle />
          {actions}
        </Space>
      )}
    </Header>
  )
}
