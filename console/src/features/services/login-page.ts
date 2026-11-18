import type { LoginPageDesign } from '@/shared/api/gen/hayat/admin/v1/login_page_pb'

/**
 * A service's login page design as the editor holds it: the proto message
 * without its type tag. Hayat checks every field again when it is saved
 * (internal/loginpage); the helpers here only make the editor honest.
 */
export type Design = Omit<LoginPageDesign, '$typeName' | '$unknown'>
export type Layout = 'centered' | 'split' | 'fullbleed'
export type Mode = 'system' | 'light' | 'dark'

export const EMPTY: Design = {
  layout: '',
  mode: '',
  accent: '',
  panel: '',
  font: '',
  radius: undefined,
  logo: '',
  logoDark: '',
  name: '',
  heading: '',
  subheading: '',
  tagline: '',
  termsUrl: '',
  privacyUrl: '',
  supportUrl: '',
  hideSignup: false,
}

export function toDesign(d: LoginPageDesign | undefined): Design {
  if (!d) return { ...EMPTY }
  const { $typeName: _t, $unknown: _u, ...rest } = d as LoginPageDesign & { $unknown?: unknown }
  return { ...EMPTY, ...rest }
}

export const sameDesign = (a: Design, b: Design) => JSON.stringify(a) === JSON.stringify(b)

export const LAYOUTS: { id: Layout; label: string; hint: string }[] = [
  { id: 'centered', label: 'Centered', hint: 'A card in the middle' },
  { id: 'split', label: 'Split', hint: 'Brand panel beside the form' },
  { id: 'fullbleed', label: 'Full-bleed', hint: 'The form on your brand' },
]

/** Font stacks, the same names as internal/loginpage.Fonts. The page downloads no fonts. */
export const FONTS: { value: string; label: string; stack: string }[] = [
  { value: 'system', label: 'System', stack: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif' },
  { value: 'humanist', label: 'Humanist', stack: 'Seravek, "Gill Sans Nova", Ubuntu, Calibri, "DejaVu Sans", sans-serif' },
  { value: 'geometric', label: 'Geometric', stack: 'Avenir, "Avenir Next", Montserrat, Corbel, "URW Gothic", sans-serif' },
  { value: 'rounded', label: 'Rounded', stack: 'ui-rounded, "Hiragino Maru Gothic ProN", Quicksand, Comfortaa, "Arial Rounded MT", sans-serif' },
  { value: 'serif', label: 'Serif', stack: 'Charter, "Bitstream Charter", "Sitka Text", Cambria, Georgia, serif' },
  { value: 'mono', label: 'Monospace', stack: 'ui-monospace, "Cascadia Code", "Source Code Pro", Menlo, Consolas, monospace' },
]

export const SWATCHES = ['#1d6a54', '#3d5fc9', '#0f7b6c', '#6d4bd8', '#c2410c', '#be185d', '#f2b134', '#14171c']

/** Hayat's accent, which an empty `accent` falls back to. */
export const DEFAULT_ACCENT = '#1d6a54'
export const DEFAULT_RADIUS = 10

// ---------- Contrast, as internal/loginpage works it out ----------

function luminance(hex: string) {
  const n = parseInt(hex.slice(1), 16)
  const ch = (v: number) => {
    v /= 255
    return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4)
  }
  return 0.2126 * ch((n >> 16) & 0xff) + 0.7152 * ch((n >> 8) & 0xff) + 0.0722 * ch(n & 0xff)
}

export function contrast(a: string, b: string) {
  const [x, y] = [luminance(a), luminance(b)].sort((p, q) => q - p)
  return (x + 0.05) / (y + 0.05)
}

/** White or near-black, whichever reads better on the fill. */
export function onColor(fill: string) {
  return contrast(fill, '#ffffff') >= contrast(fill, '#0b0d10') ? '#ffffff' : '#0b0d10'
}

export const isHex = (v: string) => /^#[0-9a-f]{6}$/i.test(v)
