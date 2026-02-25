import { useApi } from '../hooks/useApi'
import Card from '../components/Card'

function fmt(n) {
  if (!n && n !== 0) return '-'
  return Number(n).toLocaleString('ko-KR') + '원'
}

export default function Dashboard() {
  const { data, loading, error, refetch } = useApi('/api/balance')

  const profitRate = data?.profit_rate
  const profitColor =
    profitRate && profitRate !== '-'
      ? parseFloat(profitRate) > 0
        ? 'text-red-400'
        : parseFloat(profitRate) < 0
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
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6 gap-4">
          <Card title="총 평가금액" value={fmt(data?.total_eval)} />
          <Card
            title="거래가능금액"
            value={fmt(data?.tradable_amount)}
            sub="예수금총금액"
          />
          <Card
            title="출금가능금액"
            value={fmt(data?.withdrawable_amount)}
            sub="D+2 정산"
          />
          <Card title="매입 금액" value={fmt(data?.purchase_amt)} />
          <Card
            title="평가 손익"
            value={fmt(data?.eval_profit_loss)}
            sub={profitRate && profitRate !== '-' ? `수익률 ${profitRate}%` : undefined}
            className={
              profitRate && profitRate !== '-'
                ? parseFloat(profitRate) > 0
                  ? 'border-red-700'
                  : parseFloat(profitRate) < 0
                  ? 'border-blue-700'
                  : ''
                : ''
            }
          />
        </div>
      )}

      {data && (
        <p className={`mt-4 text-sm font-semibold ${profitColor}`}>
          {profitRate && profitRate !== '-'
            ? `전체 수익률: ${profitRate}%`
            : '보유 종목 없음'}
        </p>
      )}
    </div>
  )
}
