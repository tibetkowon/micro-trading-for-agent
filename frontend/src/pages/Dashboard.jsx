import { useApi } from '../hooks/useApi'
import Card from '../components/Card'

function fmt(n) {
  if (!n && n !== 0) return '-'
  return Number(n).toLocaleString('ko-KR') + '원'
}

export default function Dashboard() {
  const { data, loading, error, refetch } = useApi('/api/balance')

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-bold">대시보드</h1>
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
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
          <Card title="총 평가금액" value={fmt(data?.total_eval)} />
          <Card title="주문 가능 금액" value={fmt(data?.available_amount)} />
          <Card
            title="평가손익률"
            value={data?.profit_rate ? `${data.profit_rate}%` : '-'}
            className={
              data?.profit_rate > 0
                ? 'border-green-700'
                : data?.profit_rate < 0
                ? 'border-red-700'
                : ''
            }
          />
        </div>
      )}
    </div>
  )
}
