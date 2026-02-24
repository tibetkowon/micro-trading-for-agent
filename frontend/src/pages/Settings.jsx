import { useState } from 'react'
import { useApi } from '../hooks/useApi'

const SETTING_KEYS = [
  { key: 'KIS_APP_KEY', label: 'KIS App Key', type: 'password' },
  { key: 'KIS_APP_SECRET', label: 'KIS App Secret', type: 'password' },
  { key: 'KIS_ACCOUNT_NO', label: '계좌번호', type: 'text' },
  { key: 'KIS_ACCOUNT_TYPE', label: '계좌 유형 (01=종합, 22=선물옵션)', type: 'text' },
]

export default function Settings() {
  const { data, loading, error, refetch } = useApi('/api/settings')
  const [form, setForm] = useState({})
  const [saving, setSaving] = useState(false)
  const [saveMsg, setSaveMsg] = useState('')

  const settings = data?.settings || []
  const getVal = (key) => {
    if (form[key] !== undefined) return form[key]
    return settings.find((s) => s.key === key)?.value || ''
  }

  const handleSave = async (key) => {
    const value = form[key]
    if (!value) return
    setSaving(true)
    setSaveMsg('')
    try {
      const res = await fetch('/api/settings', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ key, value }),
      })
      if (!res.ok) {
        const b = await res.json()
        throw new Error(b.error || 'Save failed')
      }
      setSaveMsg(`${key} 저장 완료`)
      refetch()
      setForm((prev) => ({ ...prev, [key]: undefined }))
    } catch (e) {
      setSaveMsg(`오류: ${e.message}`)
    } finally {
      setSaving(false)
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
      {saveMsg && (
        <div className="bg-blue-900/30 border border-blue-700 text-blue-300 rounded p-3 mb-4 text-sm">
          {saveMsg}
        </div>
      )}

      {loading ? (
        <p className="text-gray-500">로딩 중...</p>
      ) : (
        <div className="space-y-4">
          {SETTING_KEYS.map(({ key, label, type }) => (
            <div key={key} className="bg-gray-900 border border-gray-800 rounded-lg p-4">
              <label className="block text-sm text-gray-400 mb-1">{label}</label>
              <div className="flex gap-2">
                <input
                  type={type}
                  value={getVal(key)}
                  onChange={(e) => setForm((prev) => ({ ...prev, [key]: e.target.value }))}
                  placeholder={`${key} 입력...`}
                  className="flex-1 bg-gray-800 border border-gray-700 rounded px-3 py-2 text-sm text-white placeholder-gray-600 focus:outline-none focus:border-blue-500"
                />
                <button
                  onClick={() => handleSave(key)}
                  disabled={saving || !form[key]}
                  className="px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:bg-gray-700 disabled:text-gray-500 rounded text-sm font-medium transition-colors"
                >
                  저장
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
