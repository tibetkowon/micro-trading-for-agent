import { useState } from 'react'
import { useApi } from '../hooks/useApi'

export default function Settings() {
  const { data, loading, error, refetch } = useApi('/api/settings')
  const [switching, setSwitching] = useState(false)
  const [msg, setMsg] = useState('')

  const handleToggle = async () => {
    if (!data) return
    setSwitching(true)
    setMsg('')
    try {
      const res = await fetch('/api/settings/mode', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ is_mock: !data.is_mock }),
      })
      const body = await res.json()
      if (!res.ok) throw new Error(body.error || 'Failed')
      setMsg(body.message)
      refetch()
    } catch (e) {
      setMsg(`오류: ${e.message}`)
    } finally {
      setSwitching(false)
    }
  }

  return (
    <div className="max-w-lg">
      <h1 className="text-xl font-bold mb-6">설정</h1>

      {error && (
        <div className="bg-red-900/30 border border-red-700 text-red-300 rounded p-4 mb-4 text-sm">
          {error}
        </div>
      )}
      {msg && (
        <div className="bg-blue-900/30 border border-blue-700 text-blue-300 rounded p-3 mb-4 text-sm">
          {msg}
        </div>
      )}

      {loading ? (
        <p className="text-gray-500">로딩 중...</p>
      ) : (
        <div className="space-y-4">

          {/* 모의/실전 토글 */}
          <div className="bg-gray-900 border border-gray-800 rounded-lg p-5">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium text-white">거래 모드</p>
                <p className="text-xs text-gray-500 mt-0.5">
                  {data?.is_mock ? '모의투자 환경에서 실행 중입니다' : '실전투자 환경에서 실행 중입니다'}
                </p>
              </div>
              <button
                onClick={handleToggle}
                disabled={switching}
                className={`relative inline-flex h-7 w-14 items-center rounded-full transition-colors focus:outline-none disabled:opacity-50 ${
                  data?.is_mock ? 'bg-yellow-500' : 'bg-green-600'
                }`}
              >
                <span
                  className={`inline-block h-5 w-5 transform rounded-full bg-white shadow transition-transform ${
                    data?.is_mock ? 'translate-x-1' : 'translate-x-8'
                  }`}
                />
              </button>
            </div>
            <div className="mt-3 flex gap-2">
              <span className={`text-xs px-2 py-0.5 rounded font-semibold ${data?.is_mock ? 'bg-yellow-900 text-yellow-300' : 'bg-gray-700 text-gray-400'}`}>
                모의투자
              </span>
              <span className={`text-xs px-2 py-0.5 rounded font-semibold ${!data?.is_mock ? 'bg-green-900 text-green-300' : 'bg-gray-700 text-gray-400'}`}>
                실전투자
              </span>
            </div>
          </div>

          {/* 계좌 정보 (읽기 전용) */}
          <div className="bg-gray-900 border border-gray-800 rounded-lg p-5 space-y-3">
            <p className="text-sm font-medium text-white mb-3">계좌 정보</p>

            <div className="flex justify-between text-sm">
              <span className="text-gray-500">계좌번호</span>
              <span className="font-mono text-gray-300">{data?.account_no || '-'}</span>
            </div>

            <div className="flex justify-between text-sm">
              <span className="text-gray-500">계좌 유형</span>
              <span className="text-gray-300">
                {data?.account_type === '01' ? '종합계좌 (01)' : data?.account_type === '22' ? '선물옵션 (22)' : data?.account_type || '-'}
              </span>
            </div>

            <div className="flex justify-between text-sm">
              <span className="text-gray-500">KIS API 키</span>
              <span className={`text-xs px-2 py-0.5 rounded font-semibold ${data?.kis_configured ? 'bg-green-900 text-green-300' : 'bg-red-900 text-red-300'}`}>
                {data?.kis_configured ? '설정됨' : '미설정'}
              </span>
            </div>
          </div>

          <p className="text-xs text-gray-600">
            API 키, 계좌번호 등 민감 정보는 서버의 .env 파일에서 관리합니다.
          </p>
        </div>
      )}
    </div>
  )
}
