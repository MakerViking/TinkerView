import type { SVGProps } from 'react'

// A small in-house stroke-icon set (24px grid) so the UI chrome doesn't rely on
// emoji. All icons are decorative by default (aria-hidden); buttons that contain
// only an icon must carry their own aria-label.

type P = SVGProps<SVGSVGElement>

function Base({ children, ...props }: P) {
  return (
    <svg
      viewBox="0 0 24 24"
      width="1em"
      height="1em"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.8}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      {...props}
    >
      {children}
    </svg>
  )
}

export const IconCamera = (p: P) => (
  <Base {...p}>
    <rect x="2.5" y="6.5" width="13" height="11" rx="2.5" />
    <path d="m15.5 10.5 5-2.5v8l-5-2.5" />
  </Base>
)

export const IconCameraOff = (p: P) => (
  <Base {...p}>
    <path d="M8 6.5h5A2.5 2.5 0 0 1 15.5 9v1.5l5-2.5v8l-2-1" />
    <path d="M15.5 14v1A2.5 2.5 0 0 1 13 17.5H5A2.5 2.5 0 0 1 2.5 15V9A2.5 2.5 0 0 1 5 6.5" />
    <path d="m3 3 18 18" />
  </Base>
)

export const IconFilm = (p: P) => (
  <Base {...p}>
    <rect x="3" y="4.5" width="18" height="15" rx="2.5" />
    <path d="M7 4.5v15M17 4.5v15M3 9.5h4M3 14.5h4M17 9.5h4M17 14.5h4" />
  </Base>
)

// A 3D printer: gantry frame, hotend, and the layer lines of a part being printed.
export const IconPrinter3d = (p: P) => (
  <Base {...p}>
    <path d="M3.5 3.5h17M5 3.5v13M19 3.5v13" />
    <path d="M8.5 3.5v3h7v-3" />
    <path d="M12 6.5v2.5" />
    <path d="M3.5 20.5h17" />
    <path d="M8.5 17.5h7M9.5 14.5h5" />
  </Base>
)

export const IconScanFace = (p: P) => (
  <Base {...p}>
    <path d="M3.5 8V6A2.5 2.5 0 0 1 6 3.5h2M16 3.5h2A2.5 2.5 0 0 1 20.5 6v2M20.5 16v2a2.5 2.5 0 0 1-2.5 2.5h-2M8 20.5H6A2.5 2.5 0 0 1 3.5 18v-2" />
    <path d="M8.5 9.5v1M15.5 9.5v1" />
    <path d="M8.5 14.5c.9 1 2.1 1.5 3.5 1.5s2.6-.5 3.5-1.5" />
  </Base>
)

export const IconChip = (p: P) => (
  <Base {...p}>
    <rect x="6" y="6" width="12" height="12" rx="2" />
    <rect x="10" y="10" width="4" height="4" rx="0.5" />
    <path d="M9 6V3M15 6V3M9 21v-3M15 21v-3M6 9H3M6 15H3M21 9h-3M21 15h-3" />
  </Base>
)

export const IconServer = (p: P) => (
  <Base {...p}>
    <rect x="3.5" y="4" width="17" height="7" rx="1.8" />
    <rect x="3.5" y="13" width="17" height="7" rx="1.8" />
    <path d="M7 7.5h.01M7 16.5h.01M10.5 7.5h3M10.5 16.5h3" />
  </Base>
)

export const IconBell = (p: P) => (
  <Base {...p}>
    <path d="M18 9.5a6 6 0 1 0-12 0c0 5-2 6-2 6h16s-2-1-2-6" />
    <path d="M10 19a2.2 2.2 0 0 0 4 0" />
  </Base>
)

export const IconPlus = (p: P) => (
  <Base {...p}>
    <path d="M12 5v14M5 12h14" />
  </Base>
)

export const IconX = (p: P) => (
  <Base {...p}>
    <path d="m6 6 12 12M18 6 6 18" />
  </Base>
)

export const IconDots = (p: P) => (
  <Base {...p} strokeWidth={2.6}>
    <path d="M5 12h.01M12 12h.01M19 12h.01" />
  </Base>
)

export const IconExpand = (p: P) => (
  <Base {...p}>
    <path d="M9 3.5H5A1.5 1.5 0 0 0 3.5 5v4M15 3.5h4A1.5 1.5 0 0 1 20.5 5v4M9 20.5H5A1.5 1.5 0 0 1 3.5 19v-4M15 20.5h4a1.5 1.5 0 0 0 1.5-1.5v-4" />
  </Base>
)

export const IconCopy = (p: P) => (
  <Base {...p}>
    <rect x="9" y="9" width="11" height="11" rx="2" />
    <path d="M5.5 15H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v.5" />
  </Base>
)

export const IconCheck = (p: P) => (
  <Base {...p}>
    <path d="m4.5 12.5 5 5 10-11" />
  </Base>
)

export const IconChevronDown = (p: P) => (
  <Base {...p}>
    <path d="m6 9 6 6 6-6" />
  </Base>
)

export const IconChevronRight = (p: P) => (
  <Base {...p}>
    <path d="m9 6 6 6-6 6" />
  </Base>
)

export const IconPlay = (p: P) => (
  <Base {...p}>
    <path d="M7 4.8v14.4a.6.6 0 0 0 .9.5l11.4-7.2a.6.6 0 0 0 0-1L7.9 4.3a.6.6 0 0 0-.9.5Z" />
  </Base>
)

export const IconRefresh = (p: P) => (
  <Base {...p}>
    <path d="M20 12a8 8 0 1 1-2.34-5.66" />
    <path d="M20 3v4h-4" />
  </Base>
)

export const IconTrash = (p: P) => (
  <Base {...p}>
    <path d="M4 6.5h16M9.5 6.5V5A1.5 1.5 0 0 1 11 3.5h2A1.5 1.5 0 0 1 14.5 5v1.5" />
    <path d="M6 6.5 7 19a2 2 0 0 0 2 1.5h6A2 2 0 0 0 17 19l1-12.5" />
    <path d="M10 10.5v6M14 10.5v6" />
  </Base>
)

export const IconPencil = (p: P) => (
  <Base {...p}>
    <path d="M14.5 5 19 9.5 8.5 20H4v-4.5L14.5 5Z" />
    <path d="m12.5 7 4.5 4.5" />
  </Base>
)

export const IconWarning = (p: P) => (
  <Base {...p}>
    <path d="M10.3 4.1 2.6 18a2 2 0 0 0 1.8 3h15.2a2 2 0 0 0 1.8-3L13.7 4.1a2 2 0 0 0-3.4 0Z" />
    <path d="M12 9.5v4.5M12 17.5h.01" />
  </Base>
)

export const IconOctagonStop = (p: P) => (
  <Base {...p}>
    <path d="M8.2 2.5h7.6l5.7 5.7v7.6l-5.7 5.7H8.2l-5.7-5.7V8.2l5.7-5.7Z" />
    <path d="M8.5 12h7" />
  </Base>
)

export const IconClock = (p: P) => (
  <Base {...p}>
    <circle cx="12" cy="12" r="8.5" />
    <path d="M12 7.5V12l3 2" />
  </Base>
)

export const IconExternal = (p: P) => (
  <Base {...p}>
    <path d="M14 4.5h5.5V10" />
    <path d="M19.5 4.5 11 13" />
    <path d="M19.5 14v4a2.5 2.5 0 0 1-2.5 2.5H6A2.5 2.5 0 0 1 3.5 18V7A2.5 2.5 0 0 1 6 4.5h4" />
  </Base>
)

export const IconArrowUp = (p: P) => (
  <Base {...p}>
    <path d="M12 19V5M5.5 11.5 12 5l6.5 6.5" />
  </Base>
)

export const IconArrowDown = (p: P) => (
  <Base {...p}>
    <path d="M12 5v14M5.5 12.5 12 19l6.5-6.5" />
  </Base>
)

export const IconThermometer = (p: P) => (
  <Base {...p}>
    <path d="M10 13.8V4.5a2 2 0 1 1 4 0v9.3a4.5 4.5 0 1 1-4 0Z" />
    <path d="M12 17.5h.01" />
  </Base>
)

export const IconFan = (p: P) => (
  <Base {...p}>
    <circle cx="12" cy="12" r="1.8" />
    <path d="M12 10.2c0-3.5 1.5-5.7 3.4-5.7 1.6 0 2.6 1.3 2.6 2.7 0 2-2.4 3-6 3ZM13.8 12c3.5 0 5.7 1.5 5.7 3.4 0 1.6-1.3 2.6-2.7 2.6-2 0-3-2.4-3-6ZM12 13.8c0 3.5-1.5 5.7-3.4 5.7-1.6 0-2.6-1.3-2.6-2.7 0-2 2.4-3 6-3ZM10.2 12c-3.5 0-5.7-1.5-5.7-3.4 0-1.6 1.3-2.6 2.7-2.6 2 0 3 2.4 3 6Z" />
  </Base>
)

export const IconUpload = (p: P) => (
  <Base {...p}>
    <path d="M12 16V4M7 8.5 12 3.5l5 5" />
    <path d="M4 16v2.5A1.5 1.5 0 0 0 5.5 20h13a1.5 1.5 0 0 0 1.5-1.5V16" />
  </Base>
)

export const IconPerson = (p: P) => (
  <Base {...p}>
    <circle cx="12" cy="7.5" r="3.5" />
    <path d="M5 20.5c0-3.6 3.1-6 7-6s7 2.4 7 6" />
  </Base>
)

export const IconActivity = (p: P) => (
  <Base {...p}>
    <path d="M2.5 12h4l3-7.5 5 15 3-7.5h4" />
  </Base>
)

export const IconHome = (p: P) => (
  <Base {...p}>
    <path d="m3.5 11 8.5-7.5L20.5 11" />
    <path d="M5.5 9.5V19a1.5 1.5 0 0 0 1.5 1.5h10a1.5 1.5 0 0 0 1.5-1.5V9.5" />
  </Base>
)

export const IconFile = (p: P) => (
  <Base {...p}>
    <path d="M13.5 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8.5L13.5 3Z" />
    <path d="M13.5 3v5.5H19" />
  </Base>
)

export const IconDisk = (p: P) => (
  <Base {...p}>
    <ellipse cx="12" cy="6" rx="8.5" ry="3" />
    <path d="M3.5 6v12c0 1.66 3.8 3 8.5 3s8.5-1.34 8.5-3V6" />
    <path d="M3.5 12c0 1.66 3.8 3 8.5 3s8.5-1.34 8.5-3" />
  </Base>
)

export const IconBox = (p: P) => (
  <Base {...p}>
    <path d="m12 2.5 8.5 4.75v9.5L12 21.5l-8.5-4.75v-9.5L12 2.5Z" />
    <path d="m3.5 7.25 8.5 4.75 8.5-4.75M12 12v9.5" />
  </Base>
)

export const IconZone = (p: P) => (
  <Base {...p}>
    <path d="M5 4.5 19 7l-2 12.5L4 16 5 4.5Z" strokeDasharray="3 2.4" />
    <circle cx="5" cy="4.5" r="1.4" fill="currentColor" stroke="none" />
    <circle cx="19" cy="7" r="1.4" fill="currentColor" stroke="none" />
    <circle cx="17" cy="19.5" r="1.4" fill="currentColor" stroke="none" />
    <circle cx="4" cy="16" r="1.4" fill="currentColor" stroke="none" />
  </Base>
)

export const IconSpinner = (p: P) => (
  <Base {...p} className={['animate-spin', p.className].filter(Boolean).join(' ')}>
    <path d="M12 3a9 9 0 1 0 9 9" />
  </Base>
)
