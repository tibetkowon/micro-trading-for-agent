import PropTypes from 'prop-types'

export default function Card({ title, value, sub, className = '' }) {
  return (
    <div className={`bg-gray-900 border border-gray-800 rounded-xl p-5 ${className}`}>
      <p className="text-xs text-gray-500 uppercase tracking-wider mb-1">{title}</p>
      <p className="text-2xl font-bold text-white">{value}</p>
      {sub && <p className="text-sm text-gray-400 mt-1">{sub}</p>}
    </div>
  )
}

Card.propTypes = {
  title: PropTypes.string.isRequired,
  value: PropTypes.string,
  sub: PropTypes.string,
  className: PropTypes.string,
}
