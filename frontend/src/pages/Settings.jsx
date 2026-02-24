import { useApi } from '../hooks/useApi'

export default function Settings() {
  const { data, loading, error } = useApi('/api/settings')

  return (
    <div className="max-w-lg">
      <h1 className="text-xl font-bold mb-6">설정</h1>

      {error && (
        <div className="bg-red-900/30 border border-red-700 text-red-300 rounded p-4 mb-4 text-sm">
          {error}
        </div>
      )}

      {loading ? (
        <p className="text-gray-500">로딩 중...</p>
      ) : (
        <div className="space-y-4">
          <div className="bg-gray-900 border border-gray-800 rounded-lg p-5 space-y-3">
            <p className="text-sm font-medium text-white mb-3">계좌 정보</p>

            <div className="flex justify-between text-sm">
              <span className="text-gray-500">계좌번호</span>
              <span className="font-mono text-gray-300">{data?.account_no || '-'}</span>
            </div>

            <div className="flex justify-between text-sm">
              <span className="text-gray-500">계좌 유형</span>
              <span className="text-gray-300">
                {data?.account_type === '01'
                  ? '종합계좌 (01)'
                  : data?.account_type === '22'
                  ? '선물옵션 (22)'
                  : data?.account_type || '-'}
              </span>
            </div>

            <div className="flex justify-between text-sm">
              <span className="text-gray-500">KIS API 키</span>
              <span
                className={`text-xs px-2 py-0.5 rounded font-semibold ${
                  data?.kis_configured
                    ? 'bg-green-900 text-green-300'
                    : 'bg-red-900 text-red-300'
                }`}
              >
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
