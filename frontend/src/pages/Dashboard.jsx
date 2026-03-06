import PropTypes from 'prop-types'
import { useApi } from '../hooks/useApi'
import Card from '../components/Card'

function fmt(n) {
  if (!n && n !== 0) return '-'
  return Number(n).toLocaleString('ko-KR') + '원'
}

function fmtRate(r) {
  if (r === undefined || r === null || r === '' || r === '-') return '-'
  const n = parseFloat(r)
  if (isNaN(n)) return '-'
  return (n > 0 ? '+' : '') + n.toFixed(2) + '%'
}

function fmtNum(s) {
  if (!s && s !== 0) return '-'
  const n = parseFloat(s)
  return isNaN(n) ? '-' : n.toLocaleString('ko-KR')
}

function StatusDot({ ok }) {
  return (
    <span className={`inline-block w-2 h-2 rounded-full mr-1.5 ${ok ? 'bg-green-400' : 'bg-gray-600'}`} />
  )
}
StatusDot.propTypes = { ok: PropTypes.bool }

export default function Dashboard() {
  const { data: status, loading: statusLoading, refetch: refetchStatus } = useApi('/api/server/status')
  const { data: balance, loading: balLoading, error: balError, refetch: refetchBal } = useApi('/api/balance')
  const { data: posData, loading: posLoading, refetch: refetchPos } = useApi('/api/positions')

  function refetchAll() {
    refetchStatus()
    refetchBal()
    refetchPos()
  }

  const changeRate = balance?.asset_change_rate
  const changeAmt = balance?.asset_change_amt ?? 0
  const changeColor =
    changeRate && changeRate !== '-'
      ? parseFloat(changeRate) > 0
        ? 'text-red-400'
        : parseFloat(changeRate) < 0
        ? 'text-blue-400'
        : ''
      : ''

  const holdings = posData?.positions || []

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-bold">대시보드</h1>
        <button
          onClick={refetchAll}
          className="text-sm px-3 py-1.5 bg-gray-800 hover:bg-gray-700 rounded"
        >
          새로고침
        </button>
      </div>

      {/* Server Status */}
      {!statusLoading && status && (
        <div className="bg-gray-900 border border-gray-800 rounded-lg p-4">
          <p className="text-xs text-gray-500 uppercase tracking-wider mb-3">서버 상태</p>
          <div className="flex flex-wrap gap-6">
            <div>
              <p className="text-xs text-gray-500 mb-1">장 운영</p>
              <p className="flex items-center text-sm font-semibold">
                <StatusDot ok={status.market_open} />
                {status.market_open ? '개장' : '폐장'}
              </p>
            </div>
            <div>
              <p className="text-xs text-gray-500 mb-1">WebSocket</p>
              <p className="flex items-center text-sm font-semibold">
                <StatusDot ok={status.ws_connected} />
                {status.ws_connected ? '연결됨' : '미연결'}
              </p>
            </div>
            <div>
              <p className="text-xs text-gray-500 mb-1">모니터링 포지션</p>
              <p className="text-sm font-semibold">
                <span className={status.monitored_count > 0 ? 'text-blue-400' : 'text-gray-400'}>
                  {status.monitored_count}개
                </span>
              </p>
            </div>
            <div>
              <p className="text-xs text-gray-500 mb-1">주문가능금액</p>
              <p className="text-sm font-semibold">{fmt(status.available_cash)}</p>
            </div>
            <div>
              <p className="text-xs text-gray-500 mb-1">트레이더 상태</p>
              <p className="text-sm font-semibold">
                <span className={
                  status.trader_state === 'IDLE' ? 'text-gray-400' :
                  status.trader_state === 'MONITORING' ? 'text-blue-400' :
                  'text-yellow-400'
                }>
                  {status.trader_state || 'IDLE'}
                </span>
              </p>
            </div>
          </div>
        </div>
      )}

      {/* Balance */}
      {balError && (
        <div className="bg-red-900/30 border border-red-700 text-red-300 rounded p-4 text-sm">
          {balError}
        </div>
      )}
      {!balLoading && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          <Card title="총 평가금액" value={fmt(balance?.total_eval)} />
          <Card title="출금가능금액" value={fmt(balance?.withdrawable_amount)} sub="예수금" />
          <Card
            title="자산증감액"
            value={fmt(changeAmt)}
            sub="전일 대비"
            className={changeAmt > 0 ? 'border-red-700' : changeAmt < 0 ? 'border-blue-700' : ''}
          />
          <Card
            title="자산증감수익률"
            value={fmtRate(changeRate)}
            sub="전일 대비"
            className={changeColor ? changeColor.replace('text-', 'border-') : ''}
          />
        </div>
      )}

      {/* Holdings */}
      <div>
        <p className="text-sm font-medium text-gray-400 mb-3">보유 종목</p>
        {posLoading ? (
          <p className="text-gray-500 text-sm">로딩 중...</p>
        ) : holdings.length === 0 ? (
          <p className="text-gray-500 text-sm">보유 종목 없음</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-800 text-gray-400 text-left">
                  <th className="pb-2 pr-4">종목</th>
                  <th className="pb-2 pr-4 text-right">보유수량</th>
                  <th className="pb-2 pr-4 text-right">매입평균가</th>
                  <th className="pb-2 pr-4 text-right">현재가</th>
                  <th className="pb-2 pr-4 text-right">평가손익</th>
                  <th className="pb-2 text-right">수익률</th>
                </tr>
              </thead>
              <tbody>
                {holdings.map((h) => {
                  const rate = parseFloat(h.evlu_erng_rt ?? 0)
                  const pnl = parseFloat(h.evlu_pfls_amt ?? 0)
                  const rateColor = rate > 0 ? 'text-red-400' : rate < 0 ? 'text-blue-400' : 'text-gray-400'
                  return (
                    <tr key={h.pdno} className="border-b border-gray-800/50 hover:bg-gray-900/50">
                      <td className="py-2 pr-4">
                        <span className="font-semibold">{h.prdt_name}</span>
                        <span className="ml-1.5 text-xs text-gray-500 font-mono">{h.pdno}</span>
                      </td>
                      <td className="py-2 pr-4 text-right">{fmtNum(h.hldg_qty)}주</td>
                      <td className="py-2 pr-4 text-right text-gray-300">{fmt(h.pchs_avg_pric)}</td>
                      <td className="py-2 pr-4 text-right font-semibold">{fmt(h.prpr)}</td>
                      <td className={`py-2 pr-4 text-right font-semibold ${rateColor}`}>
                        {pnl > 0 ? '+' : ''}{fmtNum(h.evlu_pfls_amt)}원
                      </td>
                      <td className={`py-2 text-right font-semibold ${rateColor}`}>
                        {rate > 0 ? '+' : ''}{rate.toFixed(2)}%
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}
