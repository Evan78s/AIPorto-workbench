import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { userAPI } from '../api'

const STATUS = { pending: '待支付', paid: '已支付', cancelled: '已取消', failed: '失败', refunded: '已退款' }

export default function Orders() {
  const [orders, setOrders] = useState([])
  const [error, setError] = useState('')
  useEffect(() => {
    userAPI.orders().then(setOrders).catch((e) => setError(e.message))
  }, [])

  return (
    <div>
      <h1>我的订单</h1>
      {error && <p className="error" role="alert">{error}</p>}
      <div className="card table-wrap">
        <table>
          <thead><tr><th>订单号</th><th>套餐</th><th>金额</th><th>状态</th><th>支付方式</th><th>下单时间</th><th>操作</th></tr></thead>
          <tbody>
            {orders.map((o) => (
              <tr key={o.order_no}>
                <td><code>{o.order_no}</code></td>
                <td>{o.plan}</td>
                <td>¥{Number(o.amount).toFixed(2)} {o.currency}</td>
                <td><span className={`badge ${o.status === 'paid' ? 'ok' : ''}`}>{STATUS[o.status] || o.status}</span></td>
                <td>{o.channel}</td>
                <td>{o.created_at}</td>
                <td><Link to={'/orders/' + encodeURIComponent(o.order_no)}>详情</Link></td>
              </tr>
            ))}
            {orders.length === 0 && <tr><td colSpan="7" className="empty">还没有订单，<Link to="/plans">去看看套餐</Link></td></tr>}
          </tbody>
        </table>
      </div>
    </div>
  )
}
