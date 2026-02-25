import PropTypes from 'prop-types'

const colors = {
  FILLED: 'bg-green-900 text-green-300',
  PARTIALLY_FILLED: 'bg-teal-900 text-teal-300',
  PENDING: 'bg-yellow-900 text-yellow-300',
  CANCELLED: 'bg-gray-700 text-gray-300',
  FAILED: 'bg-red-900 text-red-300',
}

const labels = {
  PARTIALLY_FILLED: '부분체결',
  FILLED: '체결',
  PENDING: '대기',
  CANCELLED: '취소',
  FAILED: '실패',
}

export default function StatusBadge({ status }) {
  return (
    <span className={`inline-block px-2 py-0.5 rounded text-xs font-semibold ${colors[status] || 'bg-gray-700 text-gray-300'}`}>
      {labels[status] || status}
    </span>
  )
}

StatusBadge.propTypes = {
  status: PropTypes.string.isRequired,
}
