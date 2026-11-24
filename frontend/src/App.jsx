import { Navigate, Route, Routes } from 'react-router-dom'
import AdminApp from './admin/AdminApp'
import ConsoleApp from './console/ConsoleApp'

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<Navigate to="/console" replace />} />
      <Route path="/admin/*" element={<AdminApp />} />
      <Route path="/console/*" element={<ConsoleApp />} />
      <Route path="*" element={<Navigate to="/console" replace />} />
    </Routes>
  )
}
