import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { AlertCircle, Check, ExternalLink, History, Loader2, Monitor, Smartphone } from 'lucide-react'
import { toast } from 'sonner'
import { LoginPageService, type LoginPage } from '@/shared/api/gen/hayat/admin/v1/login_page_pb'
import type { ServiceInfo } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { formatDateTime } from '@/shared/lib/datetime'
import { cn } from '@/shared/lib/utils'
import { ErrorState, LoadingState, Pill } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { LoginPageHistory } from '../components/LoginPageHistory'
import { LoginPageInspector } from '../components/LoginPageInspector'
import { toDesign, type Design } from '../login-page'

type Screen = 'login' | 'signup' | 'error'
type PreviewMode = 'design' | 'light' | 'dark'
type Device = 'desktop' | 'phone'
type SaveState = 'idle' | 'pending' | 'saving' | 'saved' | 'error'

const DEVICES: Record<Device, { w: number; h: number }> = { desktop: { w: 1280, h: 800 }, phone: { w: 390, h: 780 } }
// A preview link lives 30 minutes; fetch a fresh one a little before.
const PREVIEW_REFRESH_MS = 25 * 60 * 1000

function Toggle<T extends string>({ label, value, options, onChange }: { label: string; value: T; options: [T, React.ReactNode, string][]; onChange: (v: T) => void }) {
  return (
    <div role="group" aria-label={label} className="inline-flex gap-0.5 rounded-lg border border-border bg-card p-0.5">
      {options.map(([id, content, name]) => (
        <button
          key={id}
          type="button"
          aria-pressed={value === id}
          aria-label={name}
          title={name}
          onClick={() => onChange(id)}
          className={cn(
            'inline-flex h-7 items-center gap-1.5 rounded-md px-2.5 text-[12px] font-medium text-muted-foreground transition-colors',
            value === id && 'bg-muted text-foreground',
          )}
        >
          {content}
        </button>
      ))}
    </div>
  )
}

/** The page as Hayat draws it, in a frame scaled to fit. */
function Preview({ src, device, host }: { src: string; device: Device; host: string }) {
  const box = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(0)
  useLayoutEffect(() => {
    const el = box.current
    if (!el) return
    const ro = new ResizeObserver(([e]) => setWidth(e.contentRect.width))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  const { w, h } = DEVICES[device]
  const scale = width ? Math.min(1, width / w) : 0
  return (
    <div className="overflow-hidden rounded-xl border border-border bg-card">
      <div className="flex h-9 items-center gap-3 border-b border-border px-3">
        <span className="flex gap-1.5" aria-hidden>
          <span className="size-2.5 rounded-full bg-border" />
          <span className="size-2.5 rounded-full bg-border" />
          <span className="size-2.5 rounded-full bg-border" />
        </span>
        <span className="min-w-0 flex-1 truncate rounded-md bg-muted px-2.5 py-1 font-mono text-[11px] text-muted-foreground">{host}/login</span>
      </div>
      <div ref={box} className="flex justify-center bg-muted/40 p-0">
        <div style={{ width: w * scale, height: h * scale }} className="overflow-hidden">
          {scale > 0 && (
            <iframe
              key={device}
              title="Login page preview"
              src={src}
              sandbox=""
              style={{ width: w, height: h, transform: `scale(${scale})`, transformOrigin: '0 0', border: 0, display: 'block' }}
            />
          )}
        </div>
      </div>
    </div>
  )
}

function SaveIndicator({ state, error }: { state: SaveState; error: string }) {
  if (state === 'pending' || state === 'saving')
    return (
      <span className="inline-flex items-center gap-1.5 text-[12px] text-muted-foreground">
        <Loader2 className="size-3.5 animate-spin" /> Saving draft…
      </span>
    )
  if (state === 'saved')
    return (
      <span className="inline-flex items-center gap-1.5 text-[12px] text-muted-foreground">
        <Check className="size-3.5" /> Draft saved
      </span>
    )
  if (state === 'error')
    return (
      <span className="inline-flex max-w-[360px] items-center gap-1.5 truncate text-[12px] text-destructive" title={error}>
        <AlertCircle className="size-3.5 shrink-0" /> {error}
      </span>
    )
  return null
}

/**
 * /services/:id/login — the service's login page: the draft on the left as
 * Hayat will draw it, its settings on the right. Every change saves the draft;
 * people see it only once it is published.
 */
export function LoginPageTab({ service }: { service: ServiceInfo }) {
  const pages = useClient(LoginPageService)
  const { data, error, loading, reload } = useQuery(() => pages.getLoginPage({ serviceId: service.id }), [pages, service.id])

  const [page, setPage] = useState<LoginPage>()
  const [draft, setDraft] = useState<Design>()
  const [save, setSave] = useState<SaveState>('idle')
  const [saveError, setSaveError] = useState('')
  const [screen, setScreen] = useState<Screen>('login')
  const [mode, setMode] = useState<PreviewMode>('design')
  const [device, setDevice] = useState<Device>('desktop')
  const [frame, setFrame] = useState(0)
  const [history, setHistory] = useState(false)
  const [discarding, setDiscarding] = useState(false)
  const [publishing, setPublishing] = useState(false)

  const dirty = useRef(false)
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined)
  const seq = useRef(0)
  const fetchedAt = useRef(0)
  const latest = useRef<Design | undefined>(undefined)
  latest.current = draft

  const adopt = useCallback((p: LoginPage) => {
    setPage(p)
    fetchedAt.current = Date.now()
    setFrame((n) => n + 1)
  }, [])

  useEffect(() => {
    if (!data?.page) return
    adopt(data.page)
    setDraft(toDesign(data.page.draft))
    dirty.current = false
  }, [data, adopt])

  const saveNow = useCallback(async () => {
    clearTimeout(timer.current)
    if (!dirty.current || !latest.current) return true
    dirty.current = false
    const mine = ++seq.current
    setSave('saving')
    try {
      const res = await pages.saveLoginPageDraft({ serviceId: service.id, design: latest.current })
      if (mine === seq.current && res.page) {
        adopt(res.page)
        setSave(dirty.current ? 'pending' : 'saved')
      }
      return true
    } catch (err) {
      if (mine === seq.current) {
        setSave('error')
        setSaveError(errorMessage(err))
      }
      return false
    }
  }, [pages, service.id, adopt])

  const change = (patch: Partial<Design>) => {
    setDraft((d) => (d ? { ...d, ...patch } : d))
    dirty.current = true
    setSave('pending')
    clearTimeout(timer.current)
    timer.current = setTimeout(() => void saveNow(), 450)
  }
  useEffect(() => () => clearTimeout(timer.current), [])

  // A preview link expires; get a new one before it does.
  useEffect(() => {
    if (page && Date.now() - fetchedAt.current > PREVIEW_REFRESH_MS) void reload()
  }, [screen, mode, device, page, reload])

  const upload = async (file: File) => {
    const res = await pages.uploadLoginPageAsset({ serviceId: service.id, data: new Uint8Array(await file.arrayBuffer()) })
    return res.assetId
  }

  const publish = async () => {
    setPublishing(true)
    try {
      if (!(await saveNow())) return
      const res = await pages.publishLoginPage({ serviceId: service.id })
      if (res.page) adopt(res.page)
      toast.success(`Published. People signing in to ${service.displayName} see version ${res.page?.version}.`)
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setPublishing(false)
    }
  }

  if (error) return <ErrorState message={error} onRetry={reload} />
  if ((loading && !data) || !page || !draft) return <LoadingState />

  const base = page.previewUrl
  const src = `${base}&screen=${screen}${mode === 'design' ? '' : `&mode=${mode}`}&v=${frame}`
  const host = (() => {
    try {
      return new URL(base).host
    } catch {
      return ''
    }
  })()
  const published = !!page.published
  const busy = save === 'pending' || save === 'saving'

  return (
    <>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 flex-wrap items-center gap-2.5">
          {!published ? (
            <Pill className="border-warning/30 text-warning">Not published</Pill>
          ) : page.hasChanges ? (
            <Pill className="border-warning/30 text-warning">Unpublished changes</Pill>
          ) : (
            <Pill className="border-success/30 text-success">Live · v{page.version}</Pill>
          )}
          <span className="text-[12px] text-muted-foreground">
            {published
              ? `Version ${page.version} published ${formatDateTime(page.publishedAt)}${page.publishedBy ? ` by ${page.publishedBy.name || page.publishedBy.email}` : ''}`
              : `People see Hayat's own page until you publish.`}
          </span>
          <SaveIndicator state={save} error={saveError} />
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" size="lg" onClick={() => setHistory(true)}>
            <History /> History
          </Button>
          {published && page.hasChanges && (
            <Button variant="outline" size="lg" onClick={() => setDiscarding(true)}>
              Discard changes
            </Button>
          )}
          <Button variant="outline" size="lg" asChild>
            <a href={src} target="_blank" rel="noreferrer">
              <ExternalLink /> Open preview
            </a>
          </Button>
          <Button size="lg" disabled={!page.hasChanges || publishing || save === 'error'} onClick={publish}>
            {(publishing || (busy && publishing)) && <Loader2 className="animate-spin" />}
            Publish
          </Button>
        </div>
      </div>

      <div className="flex flex-wrap items-start gap-5">
        <div className="flex min-w-0 flex-[999_1_560px] flex-col gap-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <Toggle<Screen>
              label="Screen"
              value={screen}
              onChange={setScreen}
              options={[
                ['login', 'Sign in', 'Sign in'],
                ['error', 'Wrong password', 'Wrong password'],
                ['signup', 'Create account', 'Create account'],
              ]}
            />
            <div className="flex flex-wrap gap-2">
              <Toggle<PreviewMode>
                label="Appearance"
                value={mode}
                onChange={setMode}
                options={[
                  ['design', 'As designed', 'As designed'],
                  ['light', 'Light', 'Light'],
                  ['dark', 'Dark', 'Dark'],
                ]}
              />
              <Toggle<Device>
                label="Device"
                value={device}
                onChange={setDevice}
                options={[
                  ['desktop', <Monitor key="d" className="size-4" />, 'Desktop'],
                  ['phone', <Smartphone key="p" className="size-4" />, 'Phone'],
                ]}
              />
            </div>
          </div>
          <Preview src={src} device={device} host={host} />
          <p className="text-[12px] text-muted-foreground">
            This is the real page, drawn by Hayat from the draft. Single sign-on, password managers and Hayat's checks all work the same on it.
          </p>
        </div>

        <aside aria-label="Login page settings" className="flex-[1_1_320px] overflow-hidden rounded-xl border border-border bg-card lg:sticky lg:top-4 lg:max-w-[360px]">
          <LoginPageInspector design={draft} serviceName={page.serviceName} serviceLogo={service.logoUrl} assetBaseUrl={page.assetBaseUrl} onChange={change} onUpload={upload} />
        </aside>
      </div>

      <LoginPageHistory
        serviceId={service.id}
        live={page.version}
        open={history}
        onOpenChange={setHistory}
        onRestored={(p) => {
          adopt(p)
          setDraft(toDesign(p.draft))
          dirty.current = false
          setSave('idle')
        }}
      />
      {discarding && (
        <ConfirmDialog
          open
          onOpenChange={(o) => !o && setDiscarding(false)}
          title="Discard the draft?"
          body={`The draft goes back to version ${page.version}, the one people see now.`}
          confirm="Discard changes"
          destructive
          onConfirm={async () => {
            clearTimeout(timer.current)
            dirty.current = false
            const res = await pages.discardLoginPageDraft({ serviceId: service.id })
            if (res.page) {
              adopt(res.page)
              setDraft(toDesign(res.page.draft))
            }
            setSave('idle')
            setDiscarding(false)
          }}
        />
      )}
    </>
  )
}
