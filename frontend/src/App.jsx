import { Routes, Route, NavLink } from 'react-router-dom'
import Dashboard from './pages/Dashboard'
import Orders from './pages/Orders'
import Monitor from './pages/Monitor'
import KISLogs from './pages/KISLogs'
import Settings from './pages/Settings'
import Reports from './pages/Reports'

const navClass = ({ isActive }) =>
  `px-4 py-2 rounded text-sm font-medium transition-colors ${
    isActive
      ? 'bg-blue-600 text-white'
      : 'text-gray-400 hover:text-white hover:bg-gray-800'
  }`

export default function App() {
  return (
    <div className="min-h-screen flex flex-col">
      <nav className="bg-gray-900 border-b border-gray-800 px-6 py-3 flex items-center gap-2">
        <span className="text-white font-bold mr-6">Micro Trading</span>
        <NavLink to="/" end className={navClass}>대시보드</NavLink>
        <NavLink to="/monitor" className={navClass}>모니터</NavLink>
        <NavLink to="/orders" className={navClass}>주문 내역</NavLink>
        <NavLink to="/logs" className={navClass}>KIS 에러 로그</NavLink>
        <NavLink to="/settings" className={navClass}>설정</NavLink>
        <NavLink to="/reports" className={navClass}>리포트</NavLink>
      </nav>
      <main className="flex-1 p-6">
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/monitor" element={<Monitor />} />
          <Route path="/orders" element={<Orders />} />
          <Route path="/logs" element={<KISLogs />} />
          <Route path="/settings" element={<Settings />} />
          <Route path="/reports" element={<Reports />} />
        </Routes>
      </main>
    </div>
  )
}
