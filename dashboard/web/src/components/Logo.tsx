// TinkerView icon mark (outline style) for the app header. Square; size via
// className. Mirrors assets/tinkerview-logo.svg's icon: a viewport (dashboard)
// holding a camera lens made of print layers (3D), with a live-record dot.
export function Mark({ className = '' }: { className?: string }) {
  return (
    <svg viewBox="0 0 128 128" className={className} fill="none" aria-hidden="true">
      <defs>
        <linearGradient id="tvHdrSky" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#38bdf8" />
          <stop offset="1" stopColor="#0ea5e9" />
        </linearGradient>
        <clipPath id="tvHdrLens">
          <circle cx="64" cy="66" r="30" />
        </clipPath>
      </defs>
      <rect x="12" y="12" width="104" height="104" rx="28" stroke="url(#tvHdrSky)" strokeWidth="7" />
      <circle cx="64" cy="66" r="30" stroke="#e2e8f0" strokeWidth="5" />
      <g clipPath="url(#tvHdrLens)" stroke="#38bdf8" strokeWidth="4" strokeLinecap="round">
        <line x1="32" y1="56" x2="96" y2="56" />
        <line x1="32" y1="66" x2="96" y2="66" />
        <line x1="32" y1="76" x2="96" y2="76" />
      </g>
      <circle cx="64" cy="66" r="7" fill="#e2e8f0" />
      <circle cx="96" cy="34" r="8" fill="#f43f5e" />
    </svg>
  )
}
