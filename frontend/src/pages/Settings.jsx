import { useState } from 'react'
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

/* ── 입력 필드 ── */
function Field({ label, hint, children }) {
  return (
    <div>
      <label className="block text-xs text-gray-400 mb-1">
        {label}
        {hint && <span className="ml-1.5 text-gray-600">{hint}</span>}
      </label>
      {children}
    </div>
  )
}
Field.propTypes = { label: PropTypes.string, hint: PropTypes.string, children: PropTypes.node }

const inputCls =
  'w-full bg-gray-800 border border-gray-700 rounded px-3 py-2 text-sm text-gray-200 placeholder-gray-600 focus:outline-none focus:border-blue-500'

const REAL_URL = 'https://openapi.koreainvestment.com:9443'
const MOCK_URL = 'https://openapivts.koreainvestment.com:29443'

export default function Settings() {
  const { data, loading, error, refetch } = useApi('/api/settings')

  const [form, setForm] = useState({
    kis_app_key: '',
    kis_app_secret: '',
    kis_account_no: '',
    kis_account_type: '',
    kis_base_url: '',
    kis_hts_id: '',
    mqtt_broker_url: '',
    mqtt_client_id: '',
  })
  const [saving, setSaving] = useState(false)
  const [saveResult, setSaveResult] = useState(null) // { ok, text }

  function handleChange(e) {
    setForm((prev) => ({ ...prev, [e.target.name]: e.target.value }))
  }

  async function handleSave(e) {
    e.preventDefault()
    setSaving(true)
    setSaveResult(null)

    // 빈 필드는 전송하지 않음 (기존 값 유지)
    const body = Object.fromEntries(
      Object.entries(form).filter(([, v]) => v.trim() !== '')
    )
    if (Object.keys(body).length === 0) {
      setSaveResult({ ok: false, text: '변경할 항목을 입력하세요.' })
      setSaving(false)
      return
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
        setSaveResult({ ok: true, text: json.message })
        setForm({
          kis_app_key: '', kis_app_secret: '', kis_account_no: '',
          kis_account_type: '', kis_base_url: '', kis_hts_id: '',
          mqtt_broker_url: '', mqtt_client_id: '',
        })
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
          {saveResult.ok && (
            <span className="ml-2 text-yellow-400 font-semibold">⚠ 서버 재시작 필요</span>
          )}
        </div>
      )}

      {/* ── 편집 폼 ── */}
      <form onSubmit={handleSave} className="space-y-5">

        {/* KIS 인증 */}
        <div className="bg-gray-900 border border-gray-800 rounded-lg p-5 space-y-4">
          <p className="text-xs text-gray-500 uppercase tracking-wider">KIS API 인증</p>

          <Field label="APP KEY" hint="(입력 시 덮어쓰기)">
            <input
              name="kis_app_key"
              type="password"
              value={form.kis_app_key}
              onChange={handleChange}
              placeholder="변경할 경우 입력"
              className={inputCls}
              autoComplete="off"
            />
          </Field>

          <Field label="APP SECRET" hint="(입력 시 덮어쓰기)">
            <input
              name="kis_app_secret"
              type="password"
              value={form.kis_app_secret}
              onChange={handleChange}
              placeholder="변경할 경우 입력"
              className={inputCls}
              autoComplete="off"
            />
          </Field>

          <Field label="계좌번호" hint="앞 8자리만 (예: 12345678)">
            <input
              name="kis_account_no"
              type="text"
              value={form.kis_account_no}
              onChange={handleChange}
              placeholder="12345678"
              maxLength={8}
              className={inputCls}
            />
          </Field>

          <Field label="계좌 유형">
            <select
              name="kis_account_type"
              value={form.kis_account_type}
              onChange={handleChange}
              className={inputCls}
            >
              <option value="">— 변경 안 함 —</option>
              <option value="01">01 — 종합계좌</option>
              <option value="22">22 — 선물옵션</option>
            </select>
          </Field>

          <Field label="API Base URL">
            <select
              name="kis_base_url"
              value={form.kis_base_url}
              onChange={handleChange}
              className={inputCls}
            >
              <option value="">— 변경 안 함 —</option>
              <option value={REAL_URL}>실전투자</option>
              <option value={MOCK_URL}>모의투자</option>
            </select>
          </Field>

          <Field label="HTS ID" hint="실시간 체결통보(H0STCNI0) 수신 시 필요">
            <input
              name="kis_hts_id"
              type="text"
              value={form.kis_hts_id}
              onChange={handleChange}
              placeholder="HTS 로그인 ID"
              className={inputCls}
            />
          </Field>
        </div>

        {/* MQTT */}
        <div className="bg-gray-900 border border-gray-800 rounded-lg p-5 space-y-4">
          <p className="text-xs text-gray-500 uppercase tracking-wider">MQTT 알림</p>

          <Field label="브로커 URL">
            <input
              name="mqtt_broker_url"
              type="text"
              value={form.mqtt_broker_url}
              onChange={handleChange}
              placeholder="tcp://localhost:1883"
              className={inputCls}
            />
          </Field>

          <Field label="클라이언트 ID">
            <input
              name="mqtt_client_id"
              type="text"
              value={form.mqtt_client_id}
              onChange={handleChange}
              placeholder="micro-trading-server"
              className={inputCls}
            />
          </Field>
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
          <p className="text-xs text-gray-500 uppercase tracking-wider">현재 상태</p>

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
        설정 변경 후 서버를 재시작해야 적용됩니다. 민감 정보는 서버의 .env 파일에 저장됩니다.
      </p>
    </div>
  )
}
