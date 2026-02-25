import { useApi } from '../hooks/useApi'
import Card from '../components/Card'

function fmt(n) {
  if (!n && n !== 0) return '-'
  return Number(n).toLocaleString('ko-KR') + '원'
}

export default function Dashboard() {
  const { data, loading, error, refetch } = useApi('/api/balance')

  const changeRate = data?.asset_change_rate
  const changeAmt = data?.asset_change_amt ?? 0
  const changeColor =
    changeRate && changeRate !== '-'
      ? parseFloat(changeRate) > 0
        ? 'text-red-400'
        : parseFloat(changeRate) < 0
        ? 'text-blue-400'
        : ''
      : ''

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
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          <Card title="총 평가금액" value={fmt(data?.total_eval)} />
          <Card
            title="출금가능금액"
            value={fmt(data?.withdrawable_amount)}
            sub="예수금"
          />
          <Card
            title="자산증감액"
            value={fmt(changeAmt)}
            sub="전일 대비"
            className={
              changeAmt > 0
                ? 'border-red-700'
                : changeAmt < 0
                ? 'border-blue-700'
                : ''
            }
          />
          <Card
            title="자산증감수익률"
            value={changeRate && changeRate !== '-' ? `${changeRate}%` : '-'}
            sub="전일 대비"
            className={changeColor ? changeColor.replace('text-', 'border-') : ''}
          />
        </div>
      )}

      {data && changeRate && changeRate !== '-' && (
        <p className={`mt-4 text-sm font-semibold ${changeColor}`}>
          전일 대비: {parseFloat(changeRate) > 0 ? '+' : ''}{changeRate}%
        </p>
      )}
    </div>
  )
}
