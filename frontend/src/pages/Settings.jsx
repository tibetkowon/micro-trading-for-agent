import PropTypes from 'prop-types'
import { useApi } from '../hooks/useApi'

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
    <span
      className={`text-xs px-2 py-0.5 rounded font-semibold ${
        ok ? 'bg-green-900 text-green-300' : 'bg-gray-800 text-gray-500'
      }`}
    >
      {ok ? trueLabel : falseLabel}
    </span>
  )
}
Badge.propTypes = { ok: PropTypes.bool, trueLabel: PropTypes.string, falseLabel: PropTypes.string }

function WsBadge({ connected }) {
  return (
    <span
      className={`text-xs px-2 py-0.5 rounded font-semibold ${
        connected ? 'bg-blue-900 text-blue-300' : 'bg-gray-800 text-gray-500'
      }`}
    >
      {connected ? '연결됨' : '미연결'}
    </span>
  )
}
WsBadge.propTypes = { connected: PropTypes.bool }

export default function Settings() {
  const { data, loading, error } = useApi('/api/settings')

  return (
    <div className="max-w-lg space-y-6">
      <h1 className="text-xl font-bold">설정</h1>

      {error && (
        <div className="bg-red-900/30 border border-red-700 text-red-300 rounded p-4 text-sm">
          {error}
        </div>
      )}

      {loading ? (
        <p className="text-gray-500">로딩 중...</p>
      ) : (
        <>
          {/* 계좌 정보 */}
          <div className="bg-gray-900 border border-gray-800 rounded-lg p-5">
            <p className="text-xs text-gray-500 uppercase tracking-wider mb-3">계좌 정보</p>
            <Row label="계좌번호">
              <span className="font-mono">{data?.account_no || '-'}</span>
            </Row>
            <Row label="계좌 유형">
              {data?.account_type === '01'
                ? '종합계좌 (01)'
                : data?.account_type === '22'
                ? '선물옵션 (22)'
                : data?.account_type || '-'}
            </Row>
            <Row label="KIS API 키">
              <Badge ok={data?.kis_configured} />
            </Row>
          </div>

          {/* 실시간 연동 설정 */}
          <div className="bg-gray-900 border border-gray-800 rounded-lg p-5">
            <p className="text-xs text-gray-500 uppercase tracking-wider mb-3">실시간 연동</p>
            <Row label="KIS HTS ID (체결통보)">
              <Badge ok={data?.hts_id_configured} falseLabel="미설정 (체결통보 비활성)" />
            </Row>
            <Row label="WebSocket 연결">
              <WsBadge connected={data?.ws_connected} />
            </Row>
            <Row label="MQTT 브로커">
              <span className="font-mono text-xs">{data?.mqtt_broker_url || '-'}</span>
            </Row>
            <Row label="MQTT 클라이언트 ID">
              <span className="font-mono text-xs">{data?.mqtt_client_id || '-'}</span>
            </Row>
          </div>

          <p className="text-xs text-gray-600">
            API 키, 계좌번호 등 민감 정보는 서버의 .env 파일에서 관리합니다.
            WebSocket은 평일 08:50에 자동 연결되고 16:00에 해제됩니다.
          </p>
        </>
      )}
    </div>
  )
}
