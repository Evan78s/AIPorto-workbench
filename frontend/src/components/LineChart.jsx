export default function LineChart({ points = [], field = 'tokens', title = '用量趋势' }) {
  const values = points.map((p) => Number(p[field] || 0))
  if (!values.length) return <div className="empty">暂无数据</div>

  const width = 640
  const height = 180
  const pad = 26
  const max = Math.max(...values, 1)
  const step = (width - pad * 2) / Math.max(values.length - 1, 1)
  const coords = values.map((v, i) => [
    pad + i * step,
    height - pad - (v / max) * (height - pad * 2),
  ])
  const line = coords.map((c, i) => `${i ? 'L' : 'M'}${c[0].toFixed(1)} ${c[1].toFixed(1)}`).join(' ')
  const last = coords[coords.length - 1]
  const area = `${line} L ${last[0].toFixed(1)} ${height - pad} L ${pad} ${height - pad} Z`

  return (
    <div>
      <div className="muted" style={{ fontSize: 12, marginBottom: 6 }}>
        {title} · 峰值 {new Intl.NumberFormat('zh-CN').format(max)}
      </div>
      <svg className="chart" viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none">
        <line x1={pad} y1={height - pad} x2={width - pad} y2={height - pad} stroke="#e5e7eb" />
        <path d={area} fill="#e8efff" />
        <path d={line} fill="none" stroke="#2563eb" strokeWidth="2" />
        {coords.map((c, i) => (
          <circle key={i} cx={c[0]} cy={c[1]} r="2.5" fill="#2563eb" />
        ))}
      </svg>
      <div className="muted" style={{ fontSize: 11, display: 'flex', justifyContent: 'space-between' }}>
        <span>{points[0]?.label}</span>
        <span>{points[points.length - 1]?.label}</span>
      </div>
    </div>
  )
}
