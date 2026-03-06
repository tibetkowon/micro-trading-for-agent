import { useState, useEffect } from 'react'
import PropTypes from 'prop-types'
import { useApi } from '../hooks/useApi'

/* ── 읽기 전용 행 ── */
function Row({ label, children }) {
  return (
    <div className="flex justify-between items-center text-sm py-2 border-b border-gray-800/60 last:border-0">
      <span className="text-gray-500">{label}</span>
      <span className="text-gray-300">{children}</span>
    </div>
  )
}
Row.propTypes = { label: PropTypes.string, children: PropTypes.node }

function Badge({ ok, trueLabel = '설정됨', falseLabel = '미설정' }) {
  return (
    <span className={`text-xs px-2 py-0.5 rounded font-semibold ${ok ? 'bg-green-900 text-green-300' : 'bg-gray-800 text-gray-500'}`}>
      {ok ? trueLabel : falseLabel}
    </span>
  )
}
Badge.propTypes = { ok: PropTypes.bool, trueLabel: PropTypes.string, falseLabel: PropTypes.string }

function WsBadge({ connected }) {
  return (
    <span className={`text-xs px-2 py-0.5 rounded font-semibold ${connected ? 'bg-blue-900 text-blue-300' : 'bg-gray-800 text-gray-500'}`}>
      {connected ? '연결됨' : '미연결'}
    </span>
  )
}
WsBadge.propTypes = { connected: PropTypes.bool }

// FID_TRGT_EXLS_CLS_CODE 10자리 각 비트의 의미
const EXCL_LABELS = [
  '투자위험',
  '투자경고',
  '투자주의',
  '관리종목',
  '정리매매',
  '불성실공시',
  '우선주',
  '거래정지',
  'ETF',
  'ETN',
]

const RANKING_TYPES = [
  { value: 'volume', label: '거래량 순위' },
  { value: 'strength', label: '체결강도 순위' },
  { value: 'exec_count', label: '대량체결 순위' },
  { value: 'disparity', label: '이격도 순위' },
]

const SELL_CONDITIONS = [
  { value: 'target_pct', label: '목표가 도달 (WebSocket 실시간)' },
  { value: 'stop_pct', label: '손절가 도달 (WebSocket 실시간)' },
  { value: 'rsi_overbought', label: 'RSI 과매수' },
  { value: 'macd_bearish', label: 'MACD 데드크로스' },
]

export default function Settings() {
  const { data, loading, error, refetch } = useApi('/api/settings')

  // Trading ON/OFF
  const [tradingEnabled, setTradingEnabled] = useState(true)
  // 10개 체크박스 (각 index = 해당 비트, true = 제외)
  const [exclBits, setExclBits] = useState(Array(10).fill(true))

  // Autonomous trading settings
  const [takeProfitPct, setTakeProfitPct] = useState('3.0')
  const [stopLossPct, setStopLossPct] = useState('2.0')
  const [rankingTypes, setRankingTypes] = useState(['volume', 'strength', 'exec_count', 'disparity'])
  const [rankingPriceMin, setRankingPriceMin] = useState('5000')
  const [rankingPriceMax, setRankingPriceMax] = useState('100000')
  const [maxPositions, setMaxPositions] = useState('1')
  const [orderAmountPct, setOrderAmountPct] = useState('95')
  const [sellConditions, setSellConditions] = useState(['target_pct', 'stop_pct'])
  const [indicatorIntervalMin, setIndicatorIntervalMin] = useState('5')
  const [rsiThreshold, setRsiThreshold] = useState('70')
  const [macdBearish, setMacdBearish] = useState(false)
  const [claudeModel, setClaudeModel] = useState('claude-sonnet-4-6')

  const [saving, setSaving] = useState(false)
  const [saveResult, setSaveResult] = useState(null)

  // 서버에서 읽어온 값으로 초기화
  useEffect(() => {
    if (!data) return
    setTradingEnabled(data.trading_enabled !== false)
    const cls = data.ranking_excl_cls || '1111111111'
    setExclBits(cls.split('').map((ch) => ch === '1'))

    if (data.take_profit_pct != null) setTakeProfitPct(String(data.take_profit_pct))
    if (data.stop_loss_pct != null) setStopLossPct(String(data.stop_loss_pct))
    if (Array.isArray(data.ranking_types)) setRankingTypes(data.ranking_types)
    if (data.ranking_price_min) setRankingPriceMin(data.ranking_price_min)
    if (data.ranking_price_max) setRankingPriceMax(data.ranking_price_max)
    if (data.max_positions != null) setMaxPositions(String(data.max_positions))
    if (data.order_amount_pct != null) setOrderAmountPct(String(data.order_amount_pct))
    if (Array.isArray(data.sell_conditions)) setSellConditions(data.sell_conditions)
    if (data.indicator_check_interval_min != null) setIndicatorIntervalMin(String(data.indicator_check_interval_min))
    if (data.indicator_rsi_sell_threshold != null) setRsiThreshold(String(data.indicator_rsi_sell_threshold))
    if (data.indicator_macd_bearish_sell != null) setMacdBearish(data.indicator_macd_bearish_sell)
    if (data.claude_model) setClaudeModel(data.claude_model)
  }, [data])

  function toggleBit(i) {
    setExclBits((prev) => prev.map((v, idx) => (idx === i ? !v : v)))
  }

  function toggleRankingType(val) {
    setRankingTypes((prev) =>
      prev.includes(val) ? prev.filter((v) => v !== val) : [...prev, val]
    )
  }

  function toggleSellCondition(val) {
    setSellConditions((prev) =>
      prev.includes(val) ? prev.filter((v) => v !== val) : [...prev, val]
    )
  }

  async function handleSave(e) {
    e.preventDefault()
    setSaving(true)
    setSaveResult(null)

    const rankingExclCls = exclBits.map((b) => (b ? '1' : '0')).join('')
    const body = {
      trading_enabled: tradingEnabled,
      ranking_excl_cls: rankingExclCls,
      take_profit_pct: parseFloat(takeProfitPct) || 3.0,
      stop_loss_pct: parseFloat(stopLossPct) || 2.0,
      ranking_types: rankingTypes,
      ranking_price_min: rankingPriceMin,
      ranking_price_max: rankingPriceMax,
      max_positions: parseInt(maxPositions) || 1,
      order_amount_pct: parseFloat(orderAmountPct) || 95,
      sell_conditions: sellConditions,
      indicator_check_interval_min: parseInt(indicatorIntervalMin) || 5,
      indicator_rsi_sell_threshold: parseFloat(rsiThreshold) || 70,
      indicator_macd_bearish_sell: macdBearish,
      claude_model: claudeModel,
    }

    try {
      const res = await fetch('/api/settings', {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      const json = await res.json()
      if (!res.ok) {
        setSaveResult({ ok: false, text: json.error || '저장 실패' })
      } else {
        setSaveResult({ ok: true, text: json.message || '저장되었습니다.' })
        refetch()
      }
    } catch (err) {
      setSaveResult({ ok: false, text: err.message })
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="max-w-lg space-y-6">
      <h1 className="text-xl font-bold">설정</h1>

      {/* 저장 결과 배너 */}
      {saveResult && (
        <div className={`rounded p-3 text-sm ${saveResult.ok ? 'bg-green-900/30 border border-green-700 text-green-300' : 'bg-red-900/30 border border-red-700 text-red-300'}`}>
          {saveResult.text}
        </div>
      )}

      {/* ── 편집 폼 ── */}
      <form onSubmit={handleSave} className="space-y-5">

        {/* Trading ON/OFF */}
        <div className="bg-gray-900 border border-gray-800 rounded-lg p-5 space-y-4">
          <p className="text-xs text-gray-500 uppercase tracking-wider">거래 제어</p>

          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm text-gray-200">Trading</p>
              <p className="text-xs text-gray-500 mt-0.5">OFF 시 주문 API가 차단됩니다</p>
            </div>
            <button
              type="button"
              onClick={() => setTradingEnabled((v) => !v)}
              className={`relative inline-flex h-7 w-12 items-center rounded-full transition-colors focus:outline-none ${tradingEnabled ? 'bg-green-600' : 'bg-gray-700'}`}
            >
              <span
                className={`inline-block h-5 w-5 transform rounded-full bg-white shadow transition-transform ${tradingEnabled ? 'translate-x-6' : 'translate-x-1'}`}
              />
            </button>
          </div>
          <p className="text-xs text-center font-semibold">
            {tradingEnabled
              ? <span className="text-green-400">거래 활성화 (ON)</span>
              : <span className="text-red-400">거래 비활성화 (OFF)</span>
            }
          </p>
        </div>

        {/* 순위조회 종목 제외 필터 */}
        <div className="bg-gray-900 border border-gray-800 rounded-lg p-5 space-y-3">
          <div>
            <p className="text-xs text-gray-500 uppercase tracking-wider">순위조회 제외 종목</p>
            <p className="text-xs text-gray-600 mt-1">체크된 항목은 순위조회 결과에서 제외됩니다 (FID_TRGT_EXLS_CLS_CODE)</p>
          </div>

          <div className="grid grid-cols-2 gap-2">
            {EXCL_LABELS.map((label, i) => (
              <label key={i} className="flex items-center gap-2 cursor-pointer group">
                <input
                  type="checkbox"
                  checked={exclBits[i]}
                  onChange={() => toggleBit(i)}
                  className="w-4 h-4 rounded border-gray-600 bg-gray-800 text-blue-500 focus:ring-blue-500 focus:ring-offset-gray-900"
                />
                <span className="text-sm text-gray-300 group-hover:text-white transition-colors">{label}</span>
              </label>
            ))}
          </div>

          <p className="text-xs text-gray-600 font-mono">
            현재 값: {exclBits.map((b) => (b ? '1' : '0')).join('')}
          </p>
        </div>

        {/* 거래 파라미터 */}
        <div className="bg-gray-900 border border-gray-800 rounded-lg p-5 space-y-4">
          <p className="text-xs text-gray-500 uppercase tracking-wider">거래 파라미터</p>

          <div className="grid grid-cols-2 gap-4">
            <label className="space-y-1">
              <span className="text-xs text-gray-400">익절 기준 (%)</span>
              <input
                type="number" step="0.1" min="0.1"
                value={takeProfitPct}
                onChange={(e) => setTakeProfitPct(e.target.value)}
                className="w-full px-3 py-1.5 bg-gray-800 border border-gray-700 rounded text-sm text-gray-200"
              />
            </label>
            <label className="space-y-1">
              <span className="text-xs text-gray-400">손절 기준 (%)</span>
              <input
                type="number" step="0.1" min="0.1"
                value={stopLossPct}
                onChange={(e) => setStopLossPct(e.target.value)}
                className="w-full px-3 py-1.5 bg-gray-800 border border-gray-700 rounded text-sm text-gray-200"
              />
            </label>
            <label className="space-y-1">
              <span className="text-xs text-gray-400">최대 동시 보유 종목</span>
              <input
                type="number" step="1" min="1" max="10"
                value={maxPositions}
                onChange={(e) => setMaxPositions(e.target.value)}
                className="w-full px-3 py-1.5 bg-gray-800 border border-gray-700 rounded text-sm text-gray-200"
              />
            </label>
            <label className="space-y-1">
              <span className="text-xs text-gray-400">주문 금액 비율 (%)</span>
              <input
                type="number" step="1" min="1" max="100"
                value={orderAmountPct}
                onChange={(e) => setOrderAmountPct(e.target.value)}
                className="w-full px-3 py-1.5 bg-gray-800 border border-gray-700 rounded text-sm text-gray-200"
              />
            </label>
          </div>
        </div>

        {/* 순위 조회 설정 */}
        <div className="bg-gray-900 border border-gray-800 rounded-lg p-5 space-y-3">
          <p className="text-xs text-gray-500 uppercase tracking-wider">순위 조회 설정</p>

          <div className="grid grid-cols-2 gap-2">
            {RANKING_TYPES.map(({ value, label }) => (
              <label key={value} className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={rankingTypes.includes(value)}
                  onChange={() => toggleRankingType(value)}
                  className="w-4 h-4 rounded border-gray-600 bg-gray-800 text-blue-500"
                />
                <span className="text-sm text-gray-300">{label}</span>
              </label>
            ))}
          </div>

          <div className="grid grid-cols-2 gap-4 mt-2">
            <label className="space-y-1">
              <span className="text-xs text-gray-400">최소 주가 (원)</span>
              <input
                type="number" step="1000" min="0"
                value={rankingPriceMin}
                onChange={(e) => setRankingPriceMin(e.target.value)}
                className="w-full px-3 py-1.5 bg-gray-800 border border-gray-700 rounded text-sm text-gray-200"
              />
            </label>
            <label className="space-y-1">
              <span className="text-xs text-gray-400">최대 주가 (원)</span>
              <input
                type="number" step="1000" min="0"
                value={rankingPriceMax}
                onChange={(e) => setRankingPriceMax(e.target.value)}
                className="w-full px-3 py-1.5 bg-gray-800 border border-gray-700 rounded text-sm text-gray-200"
              />
            </label>
          </div>
        </div>

        {/* 매도 조건 */}
        <div className="bg-gray-900 border border-gray-800 rounded-lg p-5 space-y-3">
          <p className="text-xs text-gray-500 uppercase tracking-wider">매도 조건</p>
          <p className="text-xs text-gray-600">체크된 조건이 순서대로 평가됩니다</p>

          <div className="space-y-2">
            {SELL_CONDITIONS.map(({ value, label }) => (
              <label key={value} className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={sellConditions.includes(value)}
                  onChange={() => toggleSellCondition(value)}
                  className="w-4 h-4 rounded border-gray-600 bg-gray-800 text-blue-500"
                />
                <span className="text-sm text-gray-300">{label}</span>
              </label>
            ))}
          </div>
        </div>

        {/* 지표 설정 */}
        <div className="bg-gray-900 border border-gray-800 rounded-lg p-5 space-y-4">
          <p className="text-xs text-gray-500 uppercase tracking-wider">지표 설정</p>

          <div className="grid grid-cols-2 gap-4">
            <label className="space-y-1">
              <span className="text-xs text-gray-400">지표 확인 주기 (분)</span>
              <input
                type="number" step="1" min="1"
                value={indicatorIntervalMin}
                onChange={(e) => setIndicatorIntervalMin(e.target.value)}
                className="w-full px-3 py-1.5 bg-gray-800 border border-gray-700 rounded text-sm text-gray-200"
              />
            </label>
            <label className="space-y-1">
              <span className="text-xs text-gray-400">RSI 매도 기준값</span>
              <input
                type="number" step="1" min="50" max="100"
                value={rsiThreshold}
                onChange={(e) => setRsiThreshold(e.target.value)}
                className="w-full px-3 py-1.5 bg-gray-800 border border-gray-700 rounded text-sm text-gray-200"
              />
            </label>
          </div>

          <label className="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              checked={macdBearish}
              onChange={(e) => setMacdBearish(e.target.checked)}
              className="w-4 h-4 rounded border-gray-600 bg-gray-800 text-blue-500"
            />
            <span className="text-sm text-gray-300">MACD 데드크로스 시 매도</span>
          </label>
        </div>

        {/* Claude 모델 */}
        <div className="bg-gray-900 border border-gray-800 rounded-lg p-5 space-y-3">
          <p className="text-xs text-gray-500 uppercase tracking-wider">Claude AI 설정</p>
          <label className="space-y-1 block">
            <span className="text-xs text-gray-400">Claude 모델명</span>
            <input
              type="text"
              value={claudeModel}
              onChange={(e) => setClaudeModel(e.target.value)}
              className="w-full px-3 py-1.5 bg-gray-800 border border-gray-700 rounded text-sm text-gray-200 font-mono"
              placeholder="claude-sonnet-4-6"
            />
          </label>
        </div>

        <button
          type="submit"
          disabled={saving}
          className="w-full py-2.5 bg-blue-600 hover:bg-blue-700 disabled:opacity-50 rounded text-sm font-semibold transition-colors"
        >
          {saving ? '저장 중...' : '설정 저장'}
        </button>
      </form>

      {/* ── 현재 상태 표시 ── */}
      {error && (
        <div className="bg-red-900/30 border border-red-700 text-red-300 rounded p-4 text-sm">{error}</div>
      )}
      {!loading && data && (
        <div className="space-y-3">
          <p className="text-xs text-gray-500 uppercase tracking-wider">서버 정보 (읽기 전용)</p>

          <div className="bg-gray-900 border border-gray-800 rounded-lg p-5">
            <p className="text-xs text-gray-500 uppercase tracking-wider mb-3">계좌 정보</p>
            <Row label="계좌번호"><span className="font-mono">{data.account_no || '-'}</span></Row>
            <Row label="계좌 유형">
              {data.account_type === '01' ? '종합계좌 (01)' : data.account_type === '22' ? '선물옵션 (22)' : data.account_type || '-'}
            </Row>
            <Row label="KIS API 키"><Badge ok={data.kis_configured} /></Row>
            <Row label="Anthropic API 키"><Badge ok={data.anthropic_configured} /></Row>
          </div>

          <div className="bg-gray-900 border border-gray-800 rounded-lg p-5">
            <p className="text-xs text-gray-500 uppercase tracking-wider mb-3">실시간 연동</p>
            <Row label="KIS HTS ID">
              <Badge ok={data.hts_id_configured} falseLabel="미설정 (체결통보 비활성)" />
            </Row>
            <Row label="WebSocket 연결"><WsBadge connected={data.ws_connected} /></Row>
            <Row label="MQTT 브로커"><span className="font-mono text-xs">{data.mqtt_broker_url || '-'}</span></Row>
            <Row label="MQTT 클라이언트 ID"><span className="font-mono text-xs">{data.mqtt_client_id || '-'}</span></Row>
          </div>
        </div>
      )}

      <p className="text-xs text-gray-600">
        KIS API 키, 계좌 정보 등 민감 정보는 서버의 .env 파일에서 관리합니다.
      </p>
    </div>
  )
}
