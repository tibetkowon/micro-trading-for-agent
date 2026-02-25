import { useState } from 'react'
import { useApi } from '../hooks/useApi'

function fmtDate(s) {
  if (!s) return '-'
  return new Date(s).toLocaleString('ko-KR')
}

export default function KISLogs() {
  const { data, loading, error, refetch } = useApi('/api/logs/kis?limit=100')
  const [deletingIds, setDeletingIds] = useState(new Set())

  const logs = data?.logs || []

  async function handleDelete(id) {
    setDeletingIds((prev) => new Set(prev).add(id))
    try {
      await fetch(`/api/logs/kis/${id}`, { method: 'DELETE' })
      refetch()
    } finally {
      setDeletingIds((prev) => {
        const next = new Set(prev)
        next.delete(id)
        return next
      })
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-bold">KIS API 에러 로그</h1>
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
      ) : logs.length === 0 ? (
        <p className="text-gray-500">기록된 에러가 없습니다.</p>
      ) : (
        <div className="space-y-3">
          {logs.map((log) => (
            <div
              key={log.id}
              className="bg-gray-900 border border-red-900/50 rounded-lg p-4"
            >
              <div className="flex items-start justify-between gap-4 flex-wrap">
                <div>
                  <span className="text-xs bg-red-900/60 text-red-300 px-2 py-0.5 rounded font-mono mr-2">
                    {log.error_code || 'UNKNOWN'}
                  </span>
                  <span className="text-xs text-gray-500 font-mono">{log.endpoint}</span>
                </div>
                <div className="flex items-center gap-3">
                  <span className="text-xs text-gray-500">{fmtDate(log.timestamp)}</span>
                  <button
                    onClick={() => handleDelete(log.id)}
                    disabled={deletingIds.has(log.id)}
                    className="text-xs px-2 py-0.5 text-gray-500 hover:text-red-400 hover:bg-red-900/20 rounded disabled:opacity-40 transition-colors"
                  >
                    {deletingIds.has(log.id) ? '...' : '삭제'}
                  </button>
                </div>
              </div>
              {log.error_message && (
                <p className="text-sm text-gray-300 mt-2">{log.error_message}</p>
              )}
              {log.raw_response && (
                <details className="mt-2">
                  <summary className="text-xs text-gray-500 cursor-pointer hover:text-gray-300">
                    Raw Response
                  </summary>
                  <pre className="mt-1 text-xs text-gray-400 bg-gray-950 rounded p-2 overflow-x-auto whitespace-pre-wrap">
                    {log.raw_response}
                  </pre>
                </details>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
