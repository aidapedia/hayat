import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import {
  normalizeLook,
  resolveMode,
  sizeTokens,
  themeTokens,
  type AppDefaults,
  type Look,
  type Resolved,
  type TokenKey,
} from './appearance'

/**
 * The signed-in person's theme, applied to the whole console.
 *
 * A theme is the account's — one for every app they sign in to with Hayat — so
 * it is stored on the account (AccountService.GetMyAppearance, loaded by
 * SessionGate) and edited at Account → Appearance. Services read the same
 * theme through DirectoryService.GetUserAppearance.
 *
 * The last theme applied is remembered in this browser only to paint the first
 * frame, before the account answers; the account's answer always replaces it.
 */

/** Hayat as it ships — index.css's two palettes, the 16px root, 0.625rem corners, the 16.5rem sidebar. */
export const DEFAULTS: AppDefaults = {
  light: {
    accent: '#1d6a54', background: '#ffffff', surface: '#ffffff', text: '#16181c', muted: '#555b63', border: '#e3e6e3',
    sidebarBackground: '#f6f7f6', sidebarText: '#16181c', sidebarActive: '#eceeec', sidebarBorder: '#e3e6e3',
    success: '#16a34a', warning: '#b8860b', destructive: '#e03131',
  },
  dark: {
    accent: '#3fae86', background: '#080a09', surface: '#101413', text: '#f2f5f3', muted: '#a3aea8', border: '#222a27',
    sidebarBackground: '#0c0f0e', sidebarText: '#f2f5f3', sidebarActive: '#1b2120', sidebarBorder: '#222a27',
    success: '#22c55e', warning: '#d6a73a', destructive: '#ef5350',
  },
  textSize: 16,
  radius: 10,
  sidebarWidth: 264,
  contentWidth: 1280,
}

// Each design token → the custom properties in index.css it fills. The
// sidebar's own tokens are navigation here; inside it (`.app-sidebar`) the
// content tokens it also draws with answer with the --nav-* set.
const VARS: Record<TokenKey, string[]> = {
  primary: ['--primary', '--ring', '--chart-1', '--sidebar-ring'],
  primaryForeground: ['--primary-foreground', '--sidebar-primary-foreground'],
  ring: [],
  background: ['--background'],
  foreground: ['--foreground', '--card-foreground', '--popover-foreground', '--secondary-foreground', '--accent-foreground'],
  card: ['--card'],
  popover: ['--popover'],
  secondary: ['--secondary'],
  muted: ['--muted'],
  accent: ['--accent'],
  mutedForeground: ['--muted-foreground'],
  faint: ['--faint'],
  border: ['--border'],
  input: ['--input'],
  panel: [],
  nav: ['--sidebar'],
  navForeground: ['--sidebar-foreground', '--sidebar-accent-foreground', '--nav-foreground'],
  navMuted: ['--nav-muted'],
  navFaint: ['--nav-faint'],
  navActive: ['--sidebar-accent', '--nav-accent'],
  navBorder: ['--sidebar-border', '--nav-border'],
  navPrimary: ['--sidebar-primary', '--nav-primary'],
  success: ['--success'],
  warning: ['--warning'],
  destructive: ['--destructive'],
}
const COLOR_VARS = [...new Set(Object.values(VARS).flat())]

const HINT_KEY = 'hayat:appearance'
const LEGACY_KEY = 'hayat:theme' // the per-browser light/dark/system this replaced

function readHint(): Look | null {
  try {
    const legacy = localStorage.getItem(LEGACY_KEY)
    localStorage.removeItem(LEGACY_KEY)
    const hint = normalizeLook(JSON.parse(localStorage.getItem(HINT_KEY) || 'null'))
    return hint ?? normalizeLook({ mode: legacy })
  } catch {
    return null
  }
}

function writeHint(look: Look | null) {
  try {
    if (look) localStorage.setItem(HINT_KEY, JSON.stringify(look))
    else localStorage.removeItem(HINT_KEY)
  } catch {
    /* a browser refusing storage just paints the defaults first */
  }
}

// Self-hosted, and fetched only when someone picks them.
function loadFont(font: Look['font']) {
  if (font === 'inter') void import('@fontsource-variable/inter')
  if (font === 'plex') {
    void import('@fontsource/ibm-plex-sans/400.css')
    void import('@fontsource/ibm-plex-sans/500.css')
    void import('@fontsource/ibm-plex-sans/600.css')
    void import('@fontsource/ibm-plex-sans/700.css')
  }
}

function setVar(name: string, value: string | undefined) {
  const style = document.documentElement.style
  if (value) style.setProperty(name, value)
  else style.removeProperty(name)
}

/** Paint a look: the dark class, then every token it sets (and none it doesn't). */
function apply(look: Look | null, resolved: Resolved) {
  const root = document.documentElement
  root.classList.toggle('dark', resolved === 'dark')

  const tokens = themeTokens(look, DEFAULTS, resolved)
  const values = new Map<string, string>()
  for (const [key, names] of Object.entries(VARS) as [TokenKey, string[]][]) {
    const v = tokens[key]
    if (v) for (const n of names) values.set(n, v)
  }
  for (const n of COLOR_VARS) setVar(n, values.get(n))

  // The root font size is what every rem scales with; --spacing is Tailwind's
  // spacing step; the font is set on <html> because Tailwind inlines --font-sans.
  const s = sizeTokens(look)
  root.style.fontSize = s.fontSize || ''
  root.style.fontFamily = s.fontFamily || ''
  setVar('--spacing', s.spacing)
  setVar('--radius', s.radius)
  setVar('--app-sidebar-width', s.sidebarWidth)
  setVar('--app-content-width', s.contentWidth)
  loadFont(look?.font)
}

type ThemeContextValue = {
  /** What is showing: the account's theme, or one being previewed. */
  look: Look | null
  /** The account's theme. */
  saved: Look | null
  resolved: Resolved
  systemDark: boolean
  /** The account has answered. */
  loaded: boolean
  /** Show a theme without saving it; `undefined` goes back to the saved one. */
  preview: (look: Look | null | undefined) => void
  /** The account's theme, as Hayat answered it (any shape; it is normalized here). */
  receive: (appearance: unknown) => void
}

const ThemeContext = createContext<ThemeContextValue | null>(null)

const prefersDark = () => window.matchMedia('(prefers-color-scheme: dark)').matches

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [saved, setSaved] = useState<Look | null>(readHint)
  const [draft, setDraft] = useState<Look | null | undefined>(undefined)
  const [loaded, setLoaded] = useState(false)
  const [systemDark, setSystemDark] = useState(prefersDark)

  useEffect(() => {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = () => setSystemDark(mq.matches)
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])

  const look = draft === undefined ? saved : draft
  const resolved = resolveMode(look, systemDark)

  useEffect(() => apply(look, resolved), [look, resolved])

  const receive = useCallback((appearance: unknown) => {
    const next = normalizeLook(appearance)
    setSaved(next)
    writeHint(next)
    setDraft(undefined)
    setLoaded(true)
  }, [])

  const value = useMemo(
    () => ({ look, saved, resolved, systemDark, loaded, preview: setDraft, receive }),
    [look, saved, resolved, systemDark, loaded, receive],
  )
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}

export function useTheme() {
  const ctx = useContext(ThemeContext)
  if (!ctx) throw new Error('useTheme outside ThemeProvider')
  return ctx
}
