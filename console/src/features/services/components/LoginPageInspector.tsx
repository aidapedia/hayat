import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Check, ImageUp, Loader2, X } from 'lucide-react'
import { toast } from 'sonner'
import { errorMessage } from '@/shared/api/client'
import { cn } from '@/shared/lib/utils'
import { Field } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Combobox } from '@/shared/ui/combobox'
import { Input } from '@/shared/ui/input'
import { Switch } from '@/shared/ui/switch'
import {
  DEFAULT_ACCENT,
  DEFAULT_RADIUS,
  FONTS,
  LAYOUTS,
  SWATCHES,
  contrast,
  isHex,
  onColor,
  type Design,
  type Layout,
  type Mode,
} from '../login-page'

type Props = {
  design: Design
  serviceName: string
  /** The service's own logo, which the page shows when it sets none. */
  serviceLogo: string
  assetBaseUrl: string
  onChange: (patch: Partial<Design>) => void
  /** Uploads a logo and answers its asset id. */
  onUpload: (file: File) => Promise<string>
}

function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-3 border-t border-border px-4 py-4 first:border-t-0">
      <h3 className="text-[10px] font-semibold tracking-[0.08em] text-faint uppercase">{title}</h3>
      {children}
    </section>
  )
}

function Segmented<T extends string>({ label, value, options, onChange }: { label: string; value: T; options: [T, string][]; onChange: (v: T) => void }) {
  return (
    <div role="group" aria-label={label} className="inline-flex gap-0.5 rounded-lg bg-muted p-0.5">
      {options.map(([id, text]) => (
        <button
          key={id}
          type="button"
          aria-pressed={value === id}
          onClick={() => onChange(id)}
          className={cn(
            'rounded-md px-2.5 py-1 text-[12px] font-medium text-muted-foreground transition-colors',
            value === id && 'bg-background text-foreground shadow-sm',
          )}
        >
          {text}
        </button>
      ))}
    </div>
  )
}

/** A miniature of each layout, so the choice reads at a glance. */
function LayoutThumb({ id, accent }: { id: Layout; accent: string }) {
  const card = <span className="block h-7 w-5 rounded-[3px] bg-background shadow-sm ring-1 ring-border" />
  if (id === 'split')
    return (
      <span className="flex h-12 overflow-hidden rounded-md bg-muted">
        <span className="w-[42%]" style={{ background: accent }} />
        <span className="flex flex-1 items-center justify-center">{card}</span>
      </span>
    )
  if (id === 'fullbleed')
    return (
      <span className="flex h-12 items-center overflow-hidden rounded-md px-2" style={{ background: accent }}>
        {card}
      </span>
    )
  return <span className="flex h-12 items-center justify-center rounded-md bg-muted">{card}</span>
}

function ColorInput({ label, value, fallback, placeholder, onChange }: { label: string; value: string; fallback: string; placeholder?: string; onChange: (v: string) => void }) {
  const [text, setText] = useState(value)
  useEffect(() => setText(value), [value])
  const shown = value || fallback
  return (
    <div className="flex items-center gap-2">
      <label className="relative size-8 shrink-0 cursor-pointer overflow-hidden rounded-md border border-input" style={{ background: shown }}>
        <span className="sr-only">{label}</span>
        <input type="color" value={shown} onChange={(e) => onChange(e.target.value)} className="absolute inset-0 size-full cursor-pointer opacity-0" />
      </label>
      <Input
        aria-label={`${label} hex`}
        value={text}
        placeholder={placeholder ?? fallback}
        onChange={(e) => {
          setText(e.target.value)
          if (isHex(e.target.value)) onChange(e.target.value.toLowerCase())
          if (e.target.value === '') onChange('')
        }}
        className="h-8 font-mono text-[12px] uppercase placeholder:font-sans placeholder:normal-case"
      />
    </div>
  )
}

function LogoField({ label, hint, id, baseUrl, fallback, onPick, onClear }: { label: string; hint?: string; id: string; baseUrl: string; fallback?: string; onPick: (f: File) => Promise<void>; onClear: () => void }) {
  const input = useRef<HTMLInputElement>(null)
  const [busy, setBusy] = useState(false)
  const pick = async (f: File | undefined) => {
    if (!f) return
    setBusy(true)
    try {
      await onPick(f)
    } finally {
      setBusy(false)
      if (input.current) input.current.value = ''
    }
  }
  return (
    <div className="flex items-center gap-3">
      <div className="flex size-11 shrink-0 items-center justify-center overflow-hidden rounded-lg border border-dashed border-border bg-muted">
        {id ? (
          <img src={baseUrl + id} alt="" className="max-h-9 max-w-9 object-contain" />
        ) : fallback ? (
          <img src={fallback} alt="" className="max-h-9 max-w-9 object-contain opacity-70" />
        ) : (
          <ImageUp className="size-4 text-faint" />
        )}
      </div>
      <div className="min-w-0 flex-1">
        <div className="text-[13px] font-medium">{label}</div>
        <div className="truncate text-[11px] text-muted-foreground">{id ? 'Uploaded' : fallback ? "Using the service's logo" : (hint ?? 'PNG, JPEG or WebP · up to 5 MB')}</div>
      </div>
      <input ref={input} type="file" accept="image/png,image/jpeg,image/webp" className="sr-only" aria-label={`Upload ${label}`} onChange={(e) => pick(e.target.files?.[0])} />
      {id && (
        <Button variant="ghost" size="icon-sm" aria-label={`Remove ${label}`} onClick={onClear}>
          <X />
        </Button>
      )}
      <Button variant="outline" size="sm" disabled={busy} onClick={() => input.current?.click()}>
        {busy && <Loader2 className="animate-spin" />}
        {id ? 'Replace' : 'Upload'}
      </Button>
    </div>
  )
}

/** The settings beside the preview: layout, brand, words, links, sign-up. */
export function LoginPageInspector({ design: d, serviceName, serviceLogo, assetBaseUrl, onChange, onUpload }: Props) {
  const layout = (d.layout || 'centered') as Layout
  const accent = d.accent || DEFAULT_ACCENT
  const on = onColor(accent)
  const ratio = contrast(accent, on)
  const name = d.name || serviceName
  const radius = d.radius ?? DEFAULT_RADIUS
  const panelled = layout !== 'centered'

  const upload = (key: 'logo' | 'logoDark') => async (f: File) => {
    try {
      onChange({ [key]: await onUpload(f) })
    } catch (err) {
      toast.error(errorMessage(err))
    }
  }

  return (
    <div className="flex flex-col">
      <Group title="Layout">
        <div className="grid grid-cols-3 gap-2">
          {LAYOUTS.map((l) => (
            <button
              key={l.id}
              type="button"
              aria-pressed={layout === l.id}
              onClick={() => onChange({ layout: l.id === 'centered' ? '' : l.id })}
              className={cn(
                'flex flex-col gap-1.5 rounded-lg border p-1.5 text-left transition-colors hover:border-primary/50',
                layout === l.id && 'border-primary ring-1 ring-primary',
              )}
            >
              <LayoutThumb id={l.id} accent={d.panel || accent} />
              <span className="px-0.5 text-[12px] font-medium">{l.label}</span>
            </button>
          ))}
        </div>
      </Group>

      <Group title="Brand">
        <LogoField label="Logo" id={d.logo} baseUrl={assetBaseUrl} fallback={serviceLogo} onPick={upload('logo')} onClear={() => onChange({ logo: '', logoDark: '' })} />
        {d.logo && (
          <LogoField
            label="Logo on dark"
            hint="Optional: shown when the page is dark"
            id={d.logoDark}
            baseUrl={assetBaseUrl}
            onPick={upload('logoDark')}
            onClear={() => onChange({ logoDark: '' })}
          />
        )}
        <Field label="Accent" hint="Buttons, links and focus rings.">
          <div className="flex flex-wrap gap-1.5">
            {SWATCHES.map((c) => (
              <button
                key={c}
                type="button"
                aria-label={c}
                aria-pressed={accent === c}
                onClick={() => onChange({ accent: c })}
                className={cn('size-6 rounded-md ring-offset-2 ring-offset-background', accent === c && 'ring-2 ring-foreground')}
                style={{ background: c }}
              />
            ))}
          </div>
          <ColorInput label="Accent color" value={d.accent} fallback={DEFAULT_ACCENT} onChange={(v) => onChange({ accent: v })} />
          <p className="flex items-center gap-1.5 text-[11px] text-success">
            <Check className="size-3" />
            {on === '#ffffff' ? 'White' : 'Dark'} button text · {ratio.toFixed(1)}:1
          </p>
        </Field>
        {panelled && (
          <Field label="Brand panel" hint="Empty follows the accent. Its text turns white or dark to stay readable.">
            <ColorInput label="Panel color" value={d.panel} fallback={accent} placeholder="Same as accent" onChange={(v) => onChange({ panel: v })} />
          </Field>
        )}
        <Field label="Appearance">
          <Segmented<Mode>
            label="Appearance"
            value={(d.mode || 'system') as Mode}
            options={[
              ['system', 'Follow device'],
              ['light', 'Light'],
              ['dark', 'Dark'],
            ]}
            onChange={(v) => onChange({ mode: v === 'system' ? '' : v })}
          />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Font">
            <Combobox
              aria-label="Font"
              value={d.font || 'system'}
              onChange={(v) => onChange({ font: v === 'system' ? '' : v })}
              options={FONTS.map((f) => ({ value: f.value, label: f.label, icon: <span style={{ fontFamily: f.stack }}>Aa</span> }))}
            />
          </Field>
          <Field label={`Corners · ${radius}px`}>
            <input
              type="range"
              min={0}
              max={20}
              value={radius}
              aria-label="Corner radius"
              onChange={(e) => onChange({ radius: Number(e.target.value) })}
              className="h-8 w-full accent-primary"
            />
          </Field>
        </div>
      </Group>

      <Group title="Words">
        <Field label="Name" htmlFor="lp-name">
          <Input id="lp-name" value={d.name} placeholder={serviceName} maxLength={60} onChange={(e) => onChange({ name: e.target.value })} />
        </Field>
        <Field label="Heading" htmlFor="lp-heading">
          <Input id="lp-heading" value={d.heading} placeholder={`Sign in to ${name}`} maxLength={80} onChange={(e) => onChange({ heading: e.target.value })} />
        </Field>
        <Field label="Below the heading" htmlFor="lp-sub">
          <Input id="lp-sub" value={d.subheading} placeholder="Optional" maxLength={160} onChange={(e) => onChange({ subheading: e.target.value })} />
        </Field>
        {panelled && (
          <Field label="Tagline" htmlFor="lp-tagline" hint="The big line on the brand panel.">
            <Input id="lp-tagline" value={d.tagline} placeholder="Every invoice, ledger and payout in one place." maxLength={140} onChange={(e) => onChange({ tagline: e.target.value })} />
          </Field>
        )}
      </Group>

      <Group title="Links">
        {(
          [
            ['termsUrl', 'Terms'],
            ['privacyUrl', 'Privacy'],
            ['supportUrl', 'Help'],
          ] as const
        ).map(([key, label]) => (
          <Field key={key} label={label} htmlFor={`lp-${key}`}>
            <Input id={`lp-${key}`} type="url" value={d[key]} placeholder="https://" onChange={(e) => onChange({ [key]: e.target.value })} />
          </Field>
        ))}
        <p className="text-[11px] text-muted-foreground">"Secured by Hayat" is always in the footer, so people know where they're typing.</p>
      </Group>

      <Group title="Sign-up">
        <label className="flex items-start justify-between gap-3">
          <span>
            <span className="block text-[13px] font-medium">Show "Create an account"</span>
            <span className="block text-[11px] text-muted-foreground">Whether people may sign up at all is set for the whole instance.</span>
          </span>
          <Switch checked={!d.hideSignup} onCheckedChange={(v) => onChange({ hideSignup: !v })} />
        </label>
      </Group>
    </div>
  )
}
