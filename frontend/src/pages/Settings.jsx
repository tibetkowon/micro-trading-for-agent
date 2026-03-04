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

export default function Settings() {
  const { data, loading, error, refetch } = useApi('/api/settings')

  // Trading ON/OFF
  const [tradingEnabled, setTradingEnabled] = useState(true)
  // 10개 체크박스 (각 index = 해당 비트, true = 제외)
  const [exclBits, setExclBits] = useState(Array(10).fill(true))

  const [saving, setSaving] = useState(false)
  const [saveResult, setSaveResult] = useState(null)

  // 서버에서 읽어온 값으로 초기화
  useEffect(() => {
    if (!data) return
    setTradingEnabled(data.trading_enabled !== false)
    const cls = data.ranking_excl_cls || '1111111111'
    setExclBits(cls.split('').map((ch) => ch === '1'))
  }, [data])

  function toggleBit(i) {
    setExclBits((prev) => prev.map((v, idx) => (idx === i ? !v : v)))
  }

  async function handleSave(e) {
    e.preventDefault()
    setSaving(true)
    setSaveResult(null)

    const rankingExclCls = exclBits.map((b) => (b ? '1' : '0')).join('')
    const body = {
      trading_enabled: tradingEnabled,
      ranking_excl_cls: rankingExclCls,
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
