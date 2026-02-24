import { useApi } from '../hooks/useApi'
import StatusBadge from '../components/StatusBadge'

function fmtDate(s) {
  if (!s) return '-'
  return new Date(s).toLocaleString('ko-KR')
}

export default function Orders() {
  const { data, loading, error, refetch } = useApi('/api/orders?limit=100')

  const orders = data?.orders || []

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-bold">주문 내역</h1>
        <button
          onClick={refetch}
          className="text-sm px-3 py-1.5 bg-gray-800 hover:bg-gray-700 rounded"
        >
          새로고침
        </button>
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
                <th className="pb-2 pr-4">가격</th>
                <th className="pb-2 pr-4">상태</th>
                <th className="pb-2">주문시각</th>
              </tr>
            </thead>
            <tbody>
              {orders.map((o) => (
                <tr key={o.id} className="border-b border-gray-800/50 hover:bg-gray-900/50">
                  <td className="py-2 pr-4 text-gray-500">{o.id}</td>
                  <td className="py-2 pr-4 font-mono">{o.stock_code}</td>
                  <td className={`py-2 pr-4 font-semibold ${o.order_type === 'BUY' ? 'text-blue-400' : 'text-red-400'}`}>
                    {o.order_type === 'BUY' ? '매수' : '매도'}
                  </td>
                  <td className="py-2 pr-4">{o.qty.toLocaleString()}</td>
                  <td className="py-2 pr-4">{Number(o.price).toLocaleString('ko-KR')}원</td>
                  <td className="py-2 pr-4"><StatusBadge status={o.status} /></td>
                  <td className="py-2 text-gray-400">{fmtDate(o.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
