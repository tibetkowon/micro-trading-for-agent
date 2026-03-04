import { useState } from 'react'
import { useApi } from '../hooks/useApi'

function fmt(n) {
  if (!n && n !== 0) return '-'
  return Number(n).toLocaleString('ko-KR') + '원'
}

function fmtDate(s) {
  if (!s) return '-'
  return new Date(s).toLocaleString('ko-KR')
}

function pct(a, b) {
  if (!a || !b || b === 0) return null
  return ((a - b) / b * 100).toFixed(2)
}

export default function Monitor() {
  const { data, loading, error, refetch } = useApi('/api/monitor/positions')
  const [removingCodes, setRemovingCodes] = useState(new Set())

  const positions = data?.positions || []

  async function handleRemove(code) {
    setRemovingCodes((prev) => new Set(prev).add(code))
    try {
      await fetch(`/api/monitor/positions/${code}`, { method: 'DELETE' })
      refetch()
    } finally {
      setRemovingCodes((prev) => {
        const next = new Set(prev)
        next.delete(code)
        return next
      })
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-3">
          <h1 className="text-xl font-bold">실시간 모니터</h1>
          {!loading && (
            <span className="text-xs px-2 py-0.5 rounded bg-blue-900/50 text-blue-300 font-semibold">
              {positions.length}개
            </span>
          )}
        </div>
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
      ) : positions.length === 0 ? (
        <div className="text-center py-16 text-gray-500 border border-gray-800 rounded-lg">
          <p className="font-medium">모니터링 중인 포지션이 없습니다</p>
          <p className="text-sm mt-2 text-gray-600">
            주문 시 <code className="bg-gray-800 px-1 rounded">target_pct</code>와{' '}
            <code className="bg-gray-800 px-1 rounded">stop_pct</code>를 포함하면 체결 후 자동 등록됩니다.
          </p>
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-gray-800 text-gray-400 text-left">
                <th className="pb-2 pr-4">종목</th>
                <th className="pb-2 pr-4 text-right">체결가</th>
                <th className="pb-2 pr-4 text-right">목표가</th>
                <th className="pb-2 pr-4 text-right">손절가</th>
                <th className="pb-2 pr-4 text-center">목표 수익률</th>
                <th className="pb-2 pr-4 text-center">손절 비율</th>
                <th className="pb-2 pr-4">등록시각</th>
                <th className="pb-2"></th>
              </tr>
            </thead>
            <tbody>
              {positions.map((p) => {
                const targetPct = pct(p.target_price, p.filled_price)
                const stopPct = pct(p.filled_price, p.stop_price)
                const isRemoving = removingCodes.has(p.stock_code)
                return (
                  <tr
                    key={p.stock_code}
                    className="border-b border-gray-800/50 hover:bg-gray-900/50"
                  >
                    <td className="py-3 pr-4">
                      <span className="font-semibold">{p.stock_name || p.stock_code}</span>
                      {p.stock_name && (
                        <span className="ml-1.5 text-xs text-gray-500 font-mono">{p.stock_code}</span>
                      )}
                    </td>
                    <td className="py-3 pr-4 text-right text-gray-300">{fmt(p.filled_price)}</td>
                    <td className="py-3 pr-4 text-right text-green-400 font-semibold">
                      {fmt(p.target_price)}
                    </td>
                    <td className="py-3 pr-4 text-right text-red-400 font-semibold">
                      {fmt(p.stop_price)}
                    </td>
                    <td className="py-3 pr-4 text-center">
                      {targetPct !== null ? (
                        <span className="text-xs px-2 py-0.5 rounded bg-green-900/40 text-green-300 font-semibold">
                          +{targetPct}%
                        </span>
                      ) : '-'}
                    </td>
                    <td className="py-3 pr-4 text-center">
                      {stopPct !== null ? (
                        <span className="text-xs px-2 py-0.5 rounded bg-red-900/40 text-red-300 font-semibold">
                          -{stopPct}%
                        </span>
                      ) : '-'}
                    </td>
                    <td className="py-3 pr-4 text-gray-400">{fmtDate(p.created_at)}</td>
                    <td className="py-3">
                      <button
                        onClick={() => handleRemove(p.stock_code)}
                        disabled={isRemoving}
                        className="text-xs px-2 py-1 text-gray-500 hover:text-red-400 hover:bg-red-900/20 rounded disabled:opacity-40 transition-colors"
                      >
                        {isRemoving ? '...' : '해제'}
                      </button>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}

      <p className="mt-6 text-xs text-gray-600">
        목표가/손절가 도달 시 MQTT <code className="bg-gray-800 px-1 rounded">trading/alert/&#123;code&#125;</code> 토픽으로 알림이 발행됩니다.
        15:15에 서버가 전량 자동 청산합니다.
      </p>
    </div>
  )
}
