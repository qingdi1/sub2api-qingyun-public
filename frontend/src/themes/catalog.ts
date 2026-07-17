export type UiStyleId =
  | 'classic'
  | 'ink'
  | 'ocean'
  | 'aurora'
  | 'sunset'
  | 'forest'
  | 'rose'
  | 'midnight'
  | 'citrus'
  | 'slate'

export interface UiStyleDefinition {
  id: UiStyleId
  primary: string
  accent: string
  surface: string
  surfaceStrong: string
  text: string
  muted: string
  border: string
  componentTint: string
  hoverGlow: string
  shadow: string
  meshA: string
  meshB: string
  meshC: string
  glow: string
  effect: string
}

export const UI_STYLE_STORAGE_KEY = 'ui_style'
export const DEFAULT_UI_STYLE: UiStyleId = 'classic'

export const UI_STYLES: UiStyleDefinition[] = [
  {
    id: 'classic',
    primary: '#14b8a6',
    accent: '#64748b',
    surface: '#f8fafc',
    surfaceStrong: '#ffffff',
    text: '#0f172a',
    muted: '#64748b',
    border: 'rgba(20, 184, 166, 0.24)',
    componentTint: 'rgba(20, 184, 166, 0.035)',
    hoverGlow: 'rgba(20, 184, 166, 0.22)',
    shadow: '0 10px 30px rgba(15, 23, 42, 0.07)',
    meshA: 'rgba(20, 184, 166, 0.12)',
    meshB: 'rgba(6, 182, 212, 0.08)',
    meshC: 'rgba(20, 184, 166, 0.08)',
    glow: 'rgba(20, 184, 166, 0.28)',
    effect: 'soft-fade'
  },
  {
    id: 'ink',
    primary: '#286b53',
    accent: '#a63d32',
    surface: '#f6f2e7',
    surfaceStrong: '#fbfaf6',
    text: '#292720',
    muted: '#716a5f',
    border: 'rgba(87, 79, 63, 0.25)',
    componentTint: 'rgba(168, 151, 112, 0.075)',
    hoverGlow: 'rgba(40, 107, 83, 0.18)',
    shadow: '0 12px 32px rgba(62, 55, 43, 0.10)',
    meshA: 'rgba(40, 107, 83, 0.12)',
    meshB: 'rgba(166, 61, 50, 0.08)',
    meshC: 'rgba(62, 55, 43, 0.08)',
    glow: 'rgba(40, 107, 83, 0.22)',
    effect: 'ink-wash'
  },
  {
    id: 'ocean',
    primary: '#0284c7',
    accent: '#0ea5e9',
    surface: '#eff8ff',
    surfaceStrong: '#f8fcff',
    text: '#0c4a6e',
    muted: '#47748c',
    border: 'rgba(2, 132, 199, 0.24)',
    componentTint: 'rgba(14, 165, 233, 0.055)',
    hoverGlow: 'rgba(14, 165, 233, 0.30)',
    shadow: '0 12px 34px rgba(2, 132, 199, 0.10)',
    meshA: 'rgba(2, 132, 199, 0.14)',
    meshB: 'rgba(14, 165, 233, 0.10)',
    meshC: 'rgba(56, 189, 248, 0.08)',
    glow: 'rgba(14, 165, 233, 0.30)',
    effect: 'wave'
  },
  {
    id: 'aurora',
    primary: '#7c3aed',
    accent: '#22d3ee',
    surface: '#f5f3ff',
    surfaceStrong: '#faf8ff',
    text: '#2e1065',
    muted: '#6d5c91',
    border: 'rgba(124, 58, 237, 0.24)',
    componentTint: 'rgba(124, 58, 237, 0.05)',
    hoverGlow: 'rgba(34, 211, 238, 0.30)',
    shadow: '0 12px 36px rgba(91, 33, 182, 0.11)',
    meshA: 'rgba(124, 58, 237, 0.14)',
    meshB: 'rgba(34, 211, 238, 0.10)',
    meshC: 'rgba(167, 139, 250, 0.08)',
    glow: 'rgba(124, 58, 237, 0.28)',
    effect: 'aurora'
  },
  {
    id: 'sunset',
    primary: '#ea580c',
    accent: '#f59e0b',
    surface: '#fff7ed',
    surfaceStrong: '#fffaf5',
    text: '#7c2d12',
    muted: '#9a5b41',
    border: 'rgba(234, 88, 12, 0.24)',
    componentTint: 'rgba(251, 146, 60, 0.06)',
    hoverGlow: 'rgba(245, 158, 11, 0.30)',
    shadow: '0 12px 34px rgba(194, 65, 12, 0.10)',
    meshA: 'rgba(234, 88, 12, 0.14)',
    meshB: 'rgba(245, 158, 11, 0.10)',
    meshC: 'rgba(251, 146, 60, 0.08)',
    glow: 'rgba(234, 88, 12, 0.28)',
    effect: 'ember'
  },
  {
    id: 'forest',
    primary: '#15803d',
    accent: '#84cc16',
    surface: '#f3faf4',
    surfaceStrong: '#f8fcf8',
    text: '#14532d',
    muted: '#557362',
    border: 'rgba(21, 128, 61, 0.24)',
    componentTint: 'rgba(74, 222, 128, 0.05)',
    hoverGlow: 'rgba(132, 204, 22, 0.25)',
    shadow: '0 12px 34px rgba(20, 83, 45, 0.10)',
    meshA: 'rgba(21, 128, 61, 0.12)',
    meshB: 'rgba(132, 204, 22, 0.10)',
    meshC: 'rgba(74, 222, 128, 0.08)',
    glow: 'rgba(21, 128, 61, 0.26)',
    effect: 'leaf'
  },
  {
    id: 'rose',
    primary: '#db2777',
    accent: '#f43f5e',
    surface: '#fff1f5',
    surfaceStrong: '#fff7fa',
    text: '#831843',
    muted: '#9b5570',
    border: 'rgba(219, 39, 119, 0.22)',
    componentTint: 'rgba(244, 114, 182, 0.055)',
    hoverGlow: 'rgba(244, 63, 94, 0.27)',
    shadow: '0 12px 34px rgba(190, 24, 93, 0.10)',
    meshA: 'rgba(219, 39, 119, 0.12)',
    meshB: 'rgba(244, 63, 94, 0.10)',
    meshC: 'rgba(251, 113, 133, 0.08)',
    glow: 'rgba(219, 39, 119, 0.28)',
    effect: 'bloom'
  },
  {
    id: 'midnight',
    primary: '#6366f1',
    accent: '#a855f7',
    surface: '#0b1020',
    surfaceStrong: '#12182b',
    text: '#e2e8f0',
    muted: '#94a3b8',
    border: 'rgba(129, 140, 248, 0.34)',
    componentTint: 'rgba(99, 102, 241, 0.09)',
    hoverGlow: 'rgba(168, 85, 247, 0.38)',
    shadow: '0 14px 38px rgba(2, 6, 23, 0.38)',
    meshA: 'rgba(99, 102, 241, 0.18)',
    meshB: 'rgba(168, 85, 247, 0.12)',
    meshC: 'rgba(56, 189, 248, 0.08)',
    glow: 'rgba(129, 140, 248, 0.34)',
    effect: 'neon'
  },
  {
    id: 'citrus',
    primary: '#ca8a04',
    accent: '#22c55e',
    surface: '#fffbeb',
    surfaceStrong: '#fffdf5',
    text: '#713f12',
    muted: '#8a692f',
    border: 'rgba(202, 138, 4, 0.25)',
    componentTint: 'rgba(250, 204, 21, 0.07)',
    hoverGlow: 'rgba(34, 197, 94, 0.27)',
    shadow: '0 12px 34px rgba(161, 98, 7, 0.09)',
    meshA: 'rgba(202, 138, 4, 0.14)',
    meshB: 'rgba(34, 197, 94, 0.10)',
    meshC: 'rgba(250, 204, 21, 0.08)',
    glow: 'rgba(234, 179, 8, 0.28)',
    effect: 'spark'
  },
  {
    id: 'slate',
    primary: '#334155',
    accent: '#0ea5e9',
    surface: '#f1f5f9',
    surfaceStrong: '#f8fafc',
    text: '#0f172a',
    muted: '#64748b',
    border: 'rgba(51, 65, 85, 0.24)',
    componentTint: 'rgba(51, 65, 85, 0.045)',
    hoverGlow: 'rgba(14, 165, 233, 0.24)',
    shadow: '0 10px 28px rgba(15, 23, 42, 0.10)',
    meshA: 'rgba(51, 65, 85, 0.12)',
    meshB: 'rgba(14, 165, 233, 0.10)',
    meshC: 'rgba(100, 116, 139, 0.08)',
    glow: 'rgba(51, 65, 85, 0.22)',
    effect: 'grid'
  }
]

export function isUiStyleId(value: string | null | undefined): value is UiStyleId {
  return UI_STYLES.some((style) => style.id === value)
}

export function getUiStyle(id?: string | null): UiStyleDefinition {
  return UI_STYLES.find((style) => style.id === id) || UI_STYLES[0]
}

export function readStoredUiStyle(): UiStyleId {
  if (typeof window === 'undefined') return DEFAULT_UI_STYLE
  const saved = localStorage.getItem(UI_STYLE_STORAGE_KEY)
  return isUiStyleId(saved) ? saved : DEFAULT_UI_STYLE
}

export function applyUiStyle(id?: string | null): UiStyleId {
  const style = getUiStyle(id)
  if (typeof document === 'undefined') return style.id

  const root = document.documentElement
  root.dataset.uiStyle = style.id
  root.style.setProperty('--ui-primary', style.primary)
  root.style.setProperty('--ui-accent', style.accent)
  root.style.setProperty('--ui-surface', style.surface)
  root.style.setProperty('--ui-surface-strong', style.surfaceStrong)
  root.style.setProperty('--ui-text', style.text)
  root.style.setProperty('--ui-muted', style.muted)
  root.style.setProperty('--ui-border', style.border)
  root.style.setProperty('--ui-component-tint', style.componentTint)
  root.style.setProperty('--ui-hover-glow', style.hoverGlow)
  root.style.setProperty('--ui-shadow', style.shadow)
  root.style.setProperty('--ui-mesh-a', style.meshA)
  root.style.setProperty('--ui-mesh-b', style.meshB)
  root.style.setProperty('--ui-mesh-c', style.meshC)
  root.style.setProperty('--ui-glow', style.glow)
  root.style.setProperty('--ui-effect', style.effect)

  root.style.setProperty('--color-primary-500', style.primary)
  root.style.setProperty('--color-primary-600', style.primary)
  root.style.setProperty('--color-primary-400', style.accent)

  if (typeof localStorage !== 'undefined') {
    localStorage.setItem(UI_STYLE_STORAGE_KEY, style.id)
  }

  return style.id
}
