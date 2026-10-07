import type { JSX } from 'react'

interface IconProps {
  size?: number
}

function base(size: number): JSX.IntrinsicElements['svg'] {
  return {
    width: size,
    height: size,
    viewBox: '0 0 24 24',
    fill: 'none',
    stroke: 'currentColor',
    strokeWidth: 1.5,
    strokeLinecap: 'round',
    strokeLinejoin: 'round',
    'aria-hidden': true,
  }
}

export function IconPlus({ size = 16 }: IconProps) {
  return (
    <svg {...base(size)}>
      <path d="M12 5v14M5 12h14" />
    </svg>
  )
}

export function IconCopy({ size = 16 }: IconProps) {
  return (
    <svg {...base(size)}>
      <rect x="9" y="9" width="11" height="11" rx="1.5" />
      <path d="M6 15H5a1.5 1.5 0 0 1-1.5-1.5v-8A1.5 1.5 0 0 1 5 4h8A1.5 1.5 0 0 1 14.5 5.5V6" />
    </svg>
  )
}

export function IconCheck({ size = 16 }: IconProps) {
  return (
    <svg {...base(size)}>
      <path d="M5 12.5l4.5 4.5L19 7" />
    </svg>
  )
}

export function IconKey({ size = 16 }: IconProps) {
  return (
    <svg {...base(size)}>
      <circle cx="8" cy="15" r="3.5" />
      <path d="M10.5 12.5 19 4M16 4h3v3" />
    </svg>
  )
}

export function IconPower({ size = 16 }: IconProps) {
  return (
    <svg {...base(size)}>
      <path d="M12 4v8" />
      <path d="M6.6 6.8a7.5 7.5 0 1 0 10.8 0" />
    </svg>
  )
}

export function IconTrash({ size = 16 }: IconProps) {
  return (
    <svg {...base(size)}>
      <path d="M4 7h16M9 7V5h6v2M6 7l1 13h10l1-13" />
    </svg>
  )
}

export function IconUpload({ size = 16 }: IconProps) {
  return (
    <svg {...base(size)}>
      <path d="M12 16V4M7 9l5-5 5 5" />
      <path d="M4 16v2.5A1.5 1.5 0 0 0 5.5 20h13a1.5 1.5 0 0 0 1.5-1.5V16" />
    </svg>
  )
}

export function IconDownload({ size = 16 }: IconProps) {
  return (
    <svg {...base(size)}>
      <path d="M12 4v12M7 11l5 5 5-5" />
      <path d="M4 16v2.5A1.5 1.5 0 0 0 5.5 20h13a1.5 1.5 0 0 0 1.5-1.5V16" />
    </svg>
  )
}

export function IconFolderPlus({ size = 16 }: IconProps) {
  return (
    <svg {...base(size)}>
      <path d="M4 7.5A1.5 1.5 0 0 1 5.5 6h3.4l1.6 2h8A1.5 1.5 0 0 1 20 9.5v7A1.5 1.5 0 0 1 18.5 18h-13A1.5 1.5 0 0 1 4 16.5z" />
      <path d="M12 11v5M9.5 13.5h5" />
    </svg>
  )
}

export function IconPencil({ size = 16 }: IconProps) {
  return (
    <svg {...base(size)}>
      <path d="M4 20h4l10.5-10.5a2 2 0 0 0 0-2.8l-1.2-1.2a2 2 0 0 0-2.8 0L4 16z" />
      <path d="M13.5 6.5l4 4" />
    </svg>
  )
}

export function IconFolder({ size = 16 }: IconProps) {
  return (
    <svg {...base(size)}>
      <path d="M4 7.5A1.5 1.5 0 0 1 5.5 6h3.4l1.6 2h8A1.5 1.5 0 0 1 20 9.5v7A1.5 1.5 0 0 1 18.5 18h-13A1.5 1.5 0 0 1 4 16.5z" />
    </svg>
  )
}

export function IconFile({ size = 16 }: IconProps) {
  return (
    <svg {...base(size)}>
      <path d="M6 3.5h7l5 5V20a.5.5 0 0 1-.5.5h-11A.5.5 0 0 1 6 20z" />
      <path d="M13 3.5v5h5" />
    </svg>
  )
}

export function IconSettings({ size = 16 }: IconProps) {
  return (
    <svg {...base(size)}>
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
    </svg>
  )
}

export function IconLogout({ size = 16 }: IconProps) {
  return (
    <svg {...base(size)}>
      <path d="M15 5H6.5A1.5 1.5 0 0 0 5 6.5v11A1.5 1.5 0 0 0 6.5 19H15" />
      <path d="M14 12h6M17 9l3 3-3 3" />
    </svg>
  )
}

export function IconChevron({ size = 16 }: IconProps) {
  return (
    <svg {...base(size)}>
      <path d="M9 6l6 6-6 6" />
    </svg>
  )
}
