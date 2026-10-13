import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App as AntApp, ConfigProvider, theme } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import { AdminPage } from './admin/AdminPage'
import { ClientPage } from './client/ClientPage'
import { useThemeMode } from './theme'
import './styles.css'

const path = window.location.pathname
const isAdmin = path === '/admin' || path.startsWith('/admin/')

// antd 默认设计语言：只切 algorithm，不映射任何自定义 token。
function Root() {
  const { resolved } = useThemeMode()
  return (
    <ConfigProvider
      locale={zhCN}
      theme={{ algorithm: resolved === 'dark' ? theme.darkAlgorithm : theme.defaultAlgorithm }}
    >
      <AntApp>{isAdmin ? <AdminPage /> : <ClientPage />}</AntApp>
    </ConfigProvider>
  )
}

const root = document.getElementById('root')
if (root) {
  createRoot(root).render(
    <StrictMode>
      <Root />
    </StrictMode>,
  )
}
