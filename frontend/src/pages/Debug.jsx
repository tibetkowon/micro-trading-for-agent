import { useState } from 'react'
import PropTypes from 'prop-types'

const API = '/api/debug'

function Section({ title, children }) {
  return (
    <div className="bg-gray-900 border border-gray-800 rounded-lg p-4 mb-4">
      <h2 className="text-sm font-semibold text-gray-400 mb-3 uppercase tracking-wide">{title}</h2>
      {children}
    </div>
  )
}
Section.propTypes = { title: PropTypes.string, children: PropTypes.node }

function Input({ label, value, onChange, placeholder, type = 'text' }) {
  return (
    <div className="flex flex-col gap-1">
      <label className="text-xs text-gray-500">{label}</label>
      <input
        type={type}
        value={value}
        onChange={e => onChange(e.target.value)}
        placeholder={placeholder}
        className="bg-gray-800 border border-gray-700 rounded px-3 py-1.5 text-sm text-white focus:outline-none focus:border-blue-500 w-full"
      />
    </div>
  )
}
Input.propTypes = {
  label: PropTypes.string,
  value: PropTypes.string,
  onChange: PropTypes.func,
  placeholder: PropTypes.string,
  type: PropTypes.string,
}

function Btn({ onClick, children, variant = 'default', disabled = false, loading = false }) {
  const base = 'px-4 py-1.5 rounded text-sm font-medium transition-colors disabled:opacity-40 min-w-[80px]'
  const variants = {
    default: 'bg-blue-600 hover:bg-blue-700 text-white',
    danger: 'bg-red-700 hover:bg-red-800 text-white',
    gray: 'bg-gray-700 hover:bg-gray-600 text-white',
  }
  return (
    <button className={`${base} ${variants[variant]}`} onClick={onClick} disabled={disabled || loading}>
      {loading ? '처리 중...' : children}
    </button>
  )
}
Btn.propTypes = {
  onClick: PropTypes.func,
  children: PropTypes.node,
  variant: PropTypes.string,
  disabled: PropTypes.bool,
  loading: PropTypes.bool,
}

function ResultBadge({ result }) {
  if (!result) return null
  const ok = result.ok
  return (
    <span className={`text-xs px-2 py-0.5 rounded font-medium ${ok ? 'bg-green-900 text-green-300' : 'bg-red-900 text-red-300'}`}>
      {ok ? '✓ 성공' : '✗ 실패'} {result.message && `— ${result.message}`}
    </span>
  )
}
ResultBadge.propTypes = { result: PropTypes.object }

export default function Debug() {
  const [wsStatus, setWsStatus] = useState(null)
  const [priceCode, setPriceCode] = useState('')
  const [priceValue, setPriceValue] = useState('')
  const [monCode, setMonCode] = useState('')
  const [monName, setMonName] = useState('')
  const [monFilled, setMonFilled] = useState('')
  const [monTarget, setMonTarget] = useState('')
  const [monStop, setMonStop] = useState('')
  const [logs, setLogs] = useState([])

  // 섹션별 로딩/결과 상태
  const [wsLoading, setWsLoading] = useState(false)
  const [wsResult, setWsResult] = useState(null)
  const [priceLoading, setPriceLoading] = useState(false)
  const [priceResult, setPriceResult] = useState(null)
  const [monLoading, setMonLoading] = useState(false)
  const [monResult, setMonResult] = useState(null)
  const [liqLoading, setLiqLoading] = useState(false)
  const [liqResult, setLiqResult] = useState(null)

  function addLog(ok, method, path, status, body) {
    const time = new Date().toLocaleTimeString()
    const entry = { ok, text: `${time} ${method} ${path}\n< ${status} ${JSON.stringify(body, null, 2)}` }
    setLogs(prev => [entry, ...prev].slice(0, 30))
  }

  async function call(method, path, body, setLoading, setResult) {
    setLoading(true)
    setResult(null)
    try {
      const opts = { method, headers: { 'Content-Type': 'application/json' } }
      if (body) opts.body = JSON.stringify(body)
      const res = await fetch(`${API}${path}`, opts)
      const data = await res.json().catch(() => ({}))
      addLog(res.ok, method, `${API}${path}`, res.status, data)
      const message = data.message || data.error || ''
      setResult({ ok: res.ok, message })
      return { ok: res.ok, data }
    } catch (e) {
      addLog(false, method, `${API}${path}`, 'ERR', { error: e.message })
      setResult({ ok: false, message: e.message })
      return { ok: false }
    } finally {
      setLoading(false)
    }
  }

  async function wsConnect() {
    const r = await call('POST', '/ws', null, setWsLoading, setWsResult)
    if (r.ok) setWsStatus(true)
  }

  async function wsDisconnect() {
    const r = await call('DELETE', '/ws', null, setWsLoading, setWsResult)
    if (r.ok) setWsStatus(false)
  }

  async function injectPrice() {
    if (!priceCode || !priceValue) {
      setPriceResult({ ok: false, message: '종목코드와 가격을 입력하세요' })
      return
    }
    await call('POST', '/price',
      { stock_code: priceCode, price: parseFloat(priceValue) },
      setPriceLoading, setPriceResult)
  }

  async function registerMonitor() {
    if (!monCode || !monName || !monFilled || !monTarget || !monStop) {
      setMonResult({ ok: false, message: '모든 항목을 입력하세요' })
      return
    }
    await call('POST', '/monitor', {
      stock_code: monCode,
      stock_name: monName,
      filled_price: parseFloat(monFilled),
      target_pct: parseFloat(monTarget),
      stop_pct: parseFloat(monStop),
    }, setMonLoading, setMonResult)
  }

  async function liquidate() {
    if (!confirm('⚠ 실제 KIS 매도 API가 호출됩니다. 계속할까요?')) return
    await call('POST', '/liquidate', null, setLiqLoading, setLiqResult)
  }

  const wsLabel = wsStatus === null ? '알 수 없음' : wsStatus ? '연결됨' : '해제됨'
  const wsColor = wsStatus === null ? 'text-gray-500' : wsStatus ? 'text-green-400' : 'text-gray-500'

  return (
    <div className="max-w-2xl mx-auto">
      <h1 className="text-xl font-bold text-white mb-6">Debug 패널 (장 외 테스트용)</h1>

      <Section title="WebSocket 제어">
        <div className="flex items-center gap-3 flex-wrap">
          <span className={`text-sm font-medium ${wsColor}`}>● {wsLabel}</span>
          <Btn onClick={wsConnect} loading={wsLoading}>연결</Btn>
          <Btn onClick={wsDisconnect} variant="gray" loading={wsLoading}>해제</Btn>
          <ResultBadge result={wsResult} />
        </div>
      </Section>

      <Section title="포지션 등록 (테스트)">
        <div className="grid grid-cols-2 gap-3 mb-3">
          <Input label="종목코드" value={monCode} onChange={setMonCode} placeholder="005930" />
          <Input label="종목명" value={monName} onChange={setMonName} placeholder="삼성전자" />
          <Input label="체결가" value={monFilled} onChange={setMonFilled} placeholder="70000" type="number" />
          <Input label="목표 (%)" value={monTarget} onChange={setMonTarget} placeholder="3.0" type="number" />
          <Input label="손절 (%)" value={monStop} onChange={setMonStop} placeholder="2.0" type="number" />
        </div>
        <div className="flex items-center gap-3">
          <Btn onClick={registerMonitor} loading={monLoading}>등록</Btn>
          <ResultBadge result={monResult} />
        </div>
      </Section>

      <Section title="가격 이벤트 주입 → MQTT is_test:true">
        <div className="grid grid-cols-2 gap-3 mb-3">
          <Input label="종목코드" value={priceCode} onChange={setPriceCode} placeholder="005930" />
          <Input label="가격" value={priceValue} onChange={setPriceValue} placeholder="72100" type="number" />
        </div>
        <div className="flex items-center gap-3">
          <Btn onClick={injectPrice} loading={priceLoading}>주입</Btn>
          <ResultBadge result={priceResult} />
        </div>
        <p className="text-xs text-gray-600 mt-2">모니터링 중인 포지션의 목표/손절가를 초과하면 MQTT alert 발행</p>
      </Section>

      <Section title="LiquidateAll">
        <p className="text-xs text-yellow-500 mb-3">⚠ 실제 KIS 매도 API 호출됨 — 장 외 시간에는 주문이 실패합니다</p>
        <div className="flex items-center gap-3">
          <Btn onClick={liquidate} variant="danger" loading={liqLoading}>청산 실행</Btn>
          <ResultBadge result={liqResult} />
        </div>
      </Section>

      <Section title="응답 로그">
        {logs.length === 0
          ? <p className="text-xs text-gray-600">버튼을 누르면 응답이 여기에 표시됩니다.</p>
          : (
            <div className="space-y-2 max-h-80 overflow-y-auto">
              {logs.map((log, i) => (
                <pre key={i} className={`text-xs rounded p-2 whitespace-pre-wrap ${log.ok ? 'text-green-400 bg-green-950/30' : 'text-red-400 bg-red-950/30'}`}>
                  {log.text}
                </pre>
              ))}
            </div>
          )
        }
      </Section>
    </div>
  )
}
