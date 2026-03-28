import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { MainLayout } from './components/Layout/MainLayout'
import { useAuthStore } from './stores/authStore'
import Dashboard from './pages/Dashboard'
import ServerDetail from './pages/ServerDetail'
import Servers from './pages/Servers'
import Probes from './pages/Probes'
import Assets from './pages/Assets'
import Settings from './pages/Settings'
import Databases from './pages/Databases'
import DatabaseDetail from './pages/DatabaseDetail'
import Billing from './pages/Billing'
import Alerts from './pages/Alerts'
import Containers from './pages/Containers'
import Logs from './pages/Logs'
import NAS from './pages/NAS'
import NASDetail from './pages/NAS/NASDetail'
import Login from './pages/Login'
import ChangePassword from './pages/ChangePassword'
import Users from './pages/Users'

function RequireAuth({ children }: { children: React.ReactNode }) {
  const token = useAuthStore((s) => s.token)
  if (!token) return <Navigate to="/login" replace />
  return <>{children}</>
}

function RequireChangePwd({ children }: { children: React.ReactNode }) {
  const mustChangePwd = useAuthStore((s) => s.mustChangePwd)
  if (mustChangePwd) return <Navigate to="/change-password" replace />
  return <>{children}</>
}

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route path="/change-password" element={<RequireAuth><ChangePassword /></RequireAuth>} />
        <Route element={<RequireAuth><RequireChangePwd><MainLayout /></RequireChangePwd></RequireAuth>}>
          <Route path="/" element={<Dashboard />} />
          <Route path="/servers" element={<Servers />} />
          <Route path="/servers/:id" element={<ServerDetail />} />
          <Route path="/databases" element={<Databases />} />
          <Route path="/databases/:id" element={<DatabaseDetail />} />
          <Route path="/nas" element={<NAS />} />
          <Route path="/nas/:id" element={<NASDetail />} />
          <Route path="/billing" element={<Billing />} />
          <Route path="/containers" element={<Containers />} />
          <Route path="/probes" element={<Probes />} />
          <Route path="/assets" element={<Assets />} />
          <Route path="/alerts" element={<Alerts />} />
          <Route path="/logs" element={<Logs />} />
          <Route path="/settings" element={<Settings />} />
          <Route path="/users" element={<Users />} />
        </Route>
      </Routes>
    </BrowserRouter>
  )
}
