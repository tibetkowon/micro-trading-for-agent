import { useState } from 'react'
import { useApi } from '../hooks/useApi'
import StatusBadge from '../components/StatusBadge'

function fmtDate(s) {
  if (!s) return '-'
  return new Date(s).toLocaleString('ko-KR')
}

function fmtPrice(price) {
  if (!price && price !== 0) return '-'
  return Number(price).toLocaleString('ko-KR') + '원'
}

const FILLED_STATUSES = new Set(['FILLED', 'PARTIALLY_FILLED'])

export default function Orders() {
  const { data, loading, error, refetch } = useApi('/api/orders?limit=100')
  const [deletingIds, setDeletingIds] = useState(new Set())
  const [syncing, setSyncing] = useState(false)

  const orders = data?.orders || []

  async function handleSync() {
    setSyncing(true)
    try {
      await fetch('/api/orders?sync=true&limit=1')
    } finally {
      setSyncing(false)
      refetch()
    }
  }

  async function handleDelete(id) {
    setDeletingIds((prev) => new Set(prev).add(id))
    try {
      await fetch(`/api/orders/${id}`, { method: 'DELETE' })
      refetch()
    } finally {
      setDeletingIds((prev) => {
        const next = new Set(prev)
        next.delete(id)
        return next
      })
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-bold">주문 내역</h1>
        <div className="flex gap-2">
          <button
            onClick={handleSync}
            disabled={syncing}
            className="text-sm px-3 py-1.5 bg-blue-900/50 hover:bg-blue-800/50 text-blue-300 rounded disabled:opacity-50"
          >
            {syncing ? '동기화 중...' : 'KIS 동기화'}
          </button>
          <button
            onClick={refetch}
            className="text-sm px-3 py-1.5 bg-gray-800 hover:bg-gray-700 rounded"
          >
            새로고침
          </button>
        </div>
      </div>

      {error && (
        <div className="bg-red-900/30 border border-red-700 text-red-300 rounded p-4 mb-4 text-sm">
          {error}
        </div>
      )}

      {loading ? (
        <p className="text-gray-500">로딩 중...</p>
      ) : orders.length === 0 ? (
        <p className="text-gray-500">주문 내역이 없습니다.</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-gray-800 text-gray-400 text-left">
                <th className="pb-2 pr-4">ID</th>
                <th className="pb-2 pr-4">종목</th>
                <th className="pb-2 pr-4">유형</th>
                <th className="pb-2 pr-4">수량</th>
                <th className="pb-2 pr-4">주문가 / 체결가</th>
                <th className="pb-2 pr-4">상태</th>
                <th className="pb-2 pr-4">주문시각</th>
                <th className="pb-2"></th>
              </tr>
            </thead>
            <tbody>
              {orders.map((o) => {
                const isFilled = FILLED_STATUSES.has(o.status)
                const isDeleting = deletingIds.has(o.id)
                return (
                  <tr key={o.id} className="border-b border-gray-800/50 hover:bg-gray-900/50">
                    <td className="py-2 pr-4 text-gray-500">{o.id}</td>
                    <td className="py-2 pr-4">
                      <span className="font-semibold">{o.stock_name || o.stock_code}</span>
                      {o.stock_name && (
                        <span className="ml-1.5 text-xs text-gray-500 font-mono">{o.stock_code}</span>
                      )}
                    </td>
                    <td className={`py-2 pr-4 font-semibold ${o.order_type === 'BUY' ? 'text-blue-400' : 'text-red-400'}`}>
                      {o.order_type === 'BUY' ? '매수' : '매도'}
                    </td>
                    <td className="py-2 pr-4">{o.qty.toLocaleString()}</td>
                    <td className="py-2 pr-4">
                      {isFilled && o.filled_price > 0 ? (
                        <span className="text-yellow-400 font-semibold">{fmtPrice(o.filled_price)}</span>
                      ) : (
                        <span className="text-gray-300">{fmtPrice(o.price)}</span>
                      )}
                    </td>
                    <td className="py-2 pr-4"><StatusBadge status={o.status} /></td>
                    <td className="py-2 pr-4 text-gray-400">{fmtDate(o.created_at)}</td>
                    <td className="py-2">
                      <button
                        onClick={() => handleDelete(o.id)}
                        disabled={isDeleting}
                        className="text-xs px-2 py-1 text-gray-500 hover:text-red-400 hover:bg-red-900/20 rounded disabled:opacity-40 transition-colors"
                      >
                        {isDeleting ? '...' : '삭제'}
                      </button>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
