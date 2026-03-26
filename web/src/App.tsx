import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { MainLayout } from './components/Layout/MainLayout'
import { useThemeStore } from './stores/themeStore'
import { useEffect } from 'react'
import Dashboard from './pages/Dashboard'
import ServerDetail from './pages/ServerDetail'
import Servers from './pages/Servers'
import Probes from './pages/Probes'
import Assets from './pages/Assets'
import Settings from './pages/Settings'

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
          <Route path="/servers" element={<Servers />} />
          <Route path="/servers/:id" element={<ServerDetail />} />
          <Route path="/probes" element={<Probes />} />
          <Route path="/assets" element={<Assets />} />
          <Route path="/settings" element={<Settings />} />
        </Route>
      </Routes>
    </BrowserRouter>
  )
}
