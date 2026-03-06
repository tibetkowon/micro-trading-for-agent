import { useState } from 'react'
import { useApi } from '../hooks/useApi'

function fmtDate(s) {
  if (!s) return '-'
  return new Date(s).toLocaleString('ko-KR')
}

export default function Reports() {
  const { data, loading, error, refetch } = useApi('/api/reports')
  const [selectedDate, setSelectedDate] = useState(null)
  const [report, setReport] = useState(null)
  const [loadingReport, setLoadingReport] = useState(false)
  const [reportError, setReportError] = useState(null)

  const reports = data?.reports || []

  async function handleSelect(date) {
    setSelectedDate(date)
    setReport(null)
    setReportError(null)
    setLoadingReport(true)
    try {
      const res = await fetch(`/api/reports/${date}`)
      if (!res.ok) {
        const body = await res.json()
        setReportError(body.error || '리포트 로딩 실패')
        return
      }
      const body = await res.json()
      setReport(body)
    } catch (e) {
      setReportError(e.message)
    } finally {
      setLoadingReport(false)
    }
  }

  return (
    <div className="flex gap-6 h-full">
      {/* 목록 패널 */}
      <div className="w-48 flex-shrink-0">
        <div className="flex items-center justify-between mb-4">
          <h1 className="text-xl font-bold">리포트</h1>
          <button
            onClick={refetch}
            className="text-xs px-2 py-1 bg-gray-800 hover:bg-gray-700 rounded"
          >
            새로고침
          </button>
        </div>

        {error && (
          <div className="text-red-400 text-xs mb-3">{error}</div>
        )}

        {loading ? (
          <p className="text-gray-500 text-sm">로딩 중...</p>
        ) : reports.length === 0 ? (
          <p className="text-gray-500 text-sm">리포트 없음</p>
        ) : (
          <ul className="space-y-1">
            {reports.map((r) => (
              <li key={r.report_date}>
                <button
                  onClick={() => handleSelect(r.report_date)}
                  className={`w-full text-left px-3 py-2 rounded text-sm transition-colors ${
                    selectedDate === r.report_date
                      ? 'bg-blue-700 text-white'
                      : 'text-gray-300 hover:bg-gray-800'
                  }`}
                >
                  {r.report_date}
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>

      {/* 내용 패널 */}
      <div className="flex-1 min-w-0">
        {!selectedDate && (
          <div className="flex items-center justify-center h-64 text-gray-600">
            날짜를 선택하세요
          </div>
        )}

        {reportError && (
          <div className="bg-red-900/30 border border-red-700 text-red-300 rounded p-4 text-sm">
            {reportError}
          </div>
        )}

        {loadingReport && (
          <p className="text-gray-500 text-sm">리포트 로딩 중...</p>
        )}

        {report && (
          <div>
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-lg font-semibold">{report.report_date} 일일 리포트</h2>
              <span className="text-xs text-gray-500">{fmtDate(report.created_at)}</span>
            </div>
            <div className="bg-gray-900 border border-gray-800 rounded-lg p-5">
              <pre className="whitespace-pre-wrap text-sm text-gray-300 font-sans leading-relaxed">
                {report.content}
              </pre>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
