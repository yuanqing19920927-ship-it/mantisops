import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { MainLayout } from './components/Layout/MainLayout'
import { useThemeStore } from './stores/themeStore'
import { useEffect } from 'react'
import Dashboard from './pages/Dashboard'

function Placeholder({ title }: { title: string }) {
  return (
    <div style={{ color: 'var(--text-secondary)' }}>
      <h1 className="text-2xl font-bold mb-4" style={{ color: 'var(--text-primary)' }}>
        {title}
      </h1>
      <p>页面开发中...</p>
    </div>
  )
}

export default function App() {
  const theme = useThemeStore((s) => s.theme)
  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme)
  }, [theme])

  return (
    <BrowserRouter>
      <Routes>
        <Route element={<MainLayout />}>
          <Route path="/" element={<Dashboard />} />
          <Route path="/servers" element={<Placeholder title="服务器列表" />} />
          <Route path="/servers/:id" element={<Placeholder title="服务器详情" />} />
          <Route path="/probes" element={<Placeholder title="端口监控" />} />
          <Route path="/assets" element={<Placeholder title="资产信息" />} />
          <Route path="/settings" element={<Placeholder title="设置" />} />
        </Route>
      </Routes>
    </BrowserRouter>
  )
}
