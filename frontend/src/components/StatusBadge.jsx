const colors = {
  FILLED: 'bg-green-900 text-green-300',
  PENDING: 'bg-yellow-900 text-yellow-300',
  CANCELLED: 'bg-gray-700 text-gray-300',
  FAILED: 'bg-red-900 text-red-300',
}

export default function StatusBadge({ status }) {
  return (
    <span className={`inline-block px-2 py-0.5 rounded text-xs font-semibold ${colors[status] || 'bg-gray-700 text-gray-300'}`}>
      {status}
    </span>
  )
}
