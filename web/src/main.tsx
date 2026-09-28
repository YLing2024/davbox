import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { AdminPage } from './admin/AdminPage'
import { ClientPage } from './client/ClientPage'
import './styles.css'

const path = window.location.pathname
const isAdmin = path === '/admin' || path.startsWith('/admin/')

const root = document.getElementById('root')
if (root) {
  createRoot(root).render(<StrictMode>{isAdmin ? <AdminPage /> : <ClientPage />}</StrictMode>)
}
