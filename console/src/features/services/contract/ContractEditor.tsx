import { useMemo, useState, type ReactNode } from 'react'
import { toast } from 'sonner'
import { RotateCcw, Sparkles, TriangleAlert } from 'lucide-react'
import { CatalogService, RegistryService, type Contract } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { Pill } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Input } from '@/shared/ui/input'
import { Tabs, TabsList, TabsTrigger } from '@/shared/ui/tabs'
import { YamlEditor } from '../components/YamlCode'
import { summaryCount } from '../lib'
import { ContractForm } from './ContractForm'
import { baselineOf, check, draftYaml, emptyDraft, EXAMPLE, manifestYaml, summarize, toDraft, type Baseline, type Draft, type Summary } from './manifest'
import { parseYaml } from './yaml'

type Mode = 'form' | 'yaml'

const readText = (text: string): unknown => (text.trim().startsWith('{') ? JSON.parse(text) : parseYaml(text))

/** Where a new version starts: the newest one, renumbered, so publishing means "add to what's there". */
function startingPoint(svc: string, contracts: Contract[], base: Baseline): { draft: Draft; from?: Contract } {
  const latest = contracts[0]
  const version = (latest?.version ?? 0) + 1
  const from = contracts.find((c) => c.status === 'active' || c.status === 'rejected')
  if (!from) return { draft: emptyDraft(svc, version) }
  try {
    return { draft: { ...toDraft(JSON.parse(from.manifestJson), svc, base), version }, from }
  } catch {
    return { draft: emptyDraft(svc, version) }
  }
}

function ChangeGroup({ title, items }: { title: string; items: { key: string; label: ReactNode; detail?: string }[] }) {
  return (
    <div className="flex flex-col gap-1.5">
      <span className="text-[11px] font-semibold tracking-wide text-faint uppercase">{title}</span>
      {items.length === 0 ? (
        <span className="text-[12px] text-faint">No changes</span>
      ) : (
        items.map((i) => (
          <div key={i.key} className="flex flex-col gap-0.5 rounded-lg bg-success/10 px-2.5 py-2 text-[12px] shadow-[inset_3px_0_0_var(--success)]">
            <span className="break-all">
              <span className="font-mono font-bold text-success">+ </span>
              {i.label}
            </span>
            {i.detail && <span className="pl-4 text-muted-foreground">{i.detail}</span>}
          </div>
        ))
      )}
    </div>
  )
}

/** What a draft adds, grouped the way the form is. */
function Changes({ version, summary: s }: { version: number; summary: Summary | null }) {
  const n = s ? summaryCount(s) : 0
  return (
    <aside aria-labelledby="changes-h" className="flex flex-col gap-4 rounded-xl border p-4">
      <div className="flex items-center justify-between gap-2">
        <h2 id="changes-h" className="text-[14px] font-semibold">
          Changes in version {version || '…'}
        </h2>
        {n > 0 && <Pill className="border-success/30 text-success">{n} added</Pill>}
      </div>
      {!s ? (
        <p className="text-[12px] text-faint">Switch to the form to see the changes, or publish and Hayat checks the YAML.</p>
      ) : (
        <>
          <ChangeGroup
            title="Resource types"
            items={[
              ...(s.added_types ?? []).map((t) => ({ key: t, label: <span className="font-mono">{t}</span> })),
              ...Object.entries(s.added_parents ?? {}).map(([t, ps]) => ({ key: `p:${t}`, label: <span className="font-mono">{t}</span>, detail: `can also be filed under ${ps.join(', ')}` })),
            ]}
          />
          <ChangeGroup title="Permissions" items={(s.added_permissions ?? []).map((p) => ({ key: p, label: <span className="font-mono">{p}</span> }))} />
          <ChangeGroup
            title="Suggested roles"
            items={[
              ...(s.added_roles ?? []).map((r) => ({ key: r, label: <span className="font-semibold">{r}</span> })),
              ...Object.entries(s.added_role_permissions ?? {}).map(([r, ps]) => ({ key: `rp:${r}`, label: <span className="font-semibold">{r}</span>, detail: `gains ${ps.join(', ')}` })),
            ]}
          />
          <div className="border-t pt-3 text-[11px] text-faint">Nothing live is removed or changed.</div>
        </>
      )}
    </aside>
  )
}

/**
 * Publishing a contract version: a form for people, YAML for those who keep it
 * in a repo. Both publish the same manifest; the server checks it again. A
 * version goes live when published, unless it files a type under another
 * service's types, which waits for that service's owners.
 */
export function ContractEditor({
  serviceId: svc,
  contracts,
  onPublished,
  onCancel,
}: {
  serviceId: string
  contracts: Contract[]
  onPublished: (c: Contract) => unknown
  onCancel: () => void
}) {
  const registry = useClient(RegistryService)
  const catalog = useClient(CatalogService)
  const { data: cat } = useQuery(() => catalog.getCatalog({}), [catalog])
  const known = useMemo(() => (cat?.types ?? []).map((t) => ({ key: t.key, service: t.service, description: t.description })), [cat])

  const active = contracts.find((c) => c.status === 'active')
  const minVersion = contracts[0]?.version ?? 0
  const base = useMemo(() => {
    try {
      return baselineOf(active?.manifestJson, svc)
    } catch {
      return baselineOf(undefined, svc)
    }
  }, [active, svc])
  const start = useMemo(() => startingPoint(svc, contracts, base), [svc, contracts, base])
  const startYaml = useMemo(() => draftYaml(start.draft), [start])

  const [mode, setMode] = useState<Mode>('form')
  const [draft, setDraft] = useState(start.draft)
  const [text, setText] = useState(startYaml)
  const [yamlError, setYamlError] = useState('')
  const [attempted, setAttempted] = useState(false)
  const [publishing, setPublishing] = useState(false)

  const problems = useMemo(() => check(draft, known, base, minVersion), [draft, known, base, minVersion])
  const summary = useMemo(() => (mode === 'form' ? summarize(draft, base) : null), [mode, draft, base])
  const current = mode === 'form' ? draftYaml(draft) : text
  const dirty = current !== startYaml

  const switchMode = (next: Mode) => {
    if (next === mode) return
    if (next === 'yaml') {
      setText(draftYaml(draft))
      setYamlError('')
      setMode('yaml')
      return
    }
    try {
      setDraft(toDraft(readText(text), svc, base))
      setYamlError('')
      setMode('form')
    } catch (err) {
      setYamlError(`The form can't read this yet: ${(err as Error).message}. Fix it here, or publish it as written.`)
    }
  }

  const reset = () => {
    setDraft(start.draft)
    setText(startYaml)
    setYamlError('')
    setAttempted(false)
  }

  const loadExample = () => {
    const example = { ...EXAMPLE(svc), version: start.draft.version }
    if (mode === 'form') setDraft(toDraft(example, svc, base))
    else setText(manifestYaml(example))
  }

  const publish = async () => {
    if (mode === 'form' && (problems.fields.size > 0 || problems.general.length > 0)) {
      setAttempted(true)
      toast.error('Some fields need a look before publishing')
      return
    }
    setPublishing(true)
    try {
      const res = await registry.publishContract({ serviceId: svc, manifest: current })
      const c = res.contract!
      toast.success(c.status === 'active' ? `Version ${c.version} is live` : `Version ${c.version} waits for ${c.namespaces.join(', ')} to approve it`)
      await onPublished(c)
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setPublishing(false)
    }
  }

  const isEmpty = draft.types.length === 0 && draft.permissions.length === 0 && draft.roles.length === 0
  const versionErr = problems.fields.get('version')
  const version = mode === 'form' ? draft.version : start.draft.version

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="min-w-0">
          <h2 className="text-[16px] font-semibold tracking-[-0.2px]">Publish version {version || '…'}</h2>
          <p className="mt-0.5 text-[12px] text-muted-foreground">It goes live as soon as you publish. Contracts only add, so check the changes first.</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Tabs value={mode} onValueChange={(v) => switchMode(v as Mode)}>
            <TabsList>
              <TabsTrigger value="form" className="px-3 text-[12px]">
                Form
              </TabsTrigger>
              <TabsTrigger value="yaml" className="px-3 text-[12px]">
                YAML
              </TabsTrigger>
            </TabsList>
          </Tabs>
          <Button size="lg" variant="outline" onClick={onCancel}>
            Cancel
          </Button>
          <Button size="lg" onClick={publish} disabled={publishing || !dirty || !current.trim()}>
            {publishing ? 'Publishing…' : 'Publish'}
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-[minmax(0,1fr)_minmax(260px,320px)] items-start gap-7 max-[1000px]:grid-cols-1">
        <div className="min-w-0 rounded-xl border">
          <div className="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
            <div className="flex flex-wrap items-center gap-3">
              {mode === 'form' && (
                <label className="flex items-center gap-2 text-[12px] font-semibold text-muted-foreground">
                  Version
                  <Input
                    type="number"
                    min={minVersion + 1}
                    value={draft.version || ''}
                    onChange={(e) => setDraft({ ...draft, version: Number(e.target.value) })}
                    aria-invalid={!!versionErr}
                    className="h-8 w-20 text-[13px]"
                  />
                </label>
              )}
              <span className="text-[12px] text-faint">
                {start.from ? `Starting from version ${start.from.version} (${start.from.status})` : 'The first version of this contract'}
              </span>
              {mode === 'form' && versionErr && <span className="text-[12px] text-destructive">{versionErr.message}</span>}
            </div>
            <div className="flex items-center gap-2">
              {!start.from && (mode === 'yaml' || isEmpty) && (
                <Button type="button" variant="ghost" size="sm" onClick={loadExample}>
                  <Sparkles /> Start from an example
                </Button>
              )}
              {dirty && (
                <Button type="button" variant="ghost" size="sm" onClick={reset}>
                  <RotateCcw /> Discard changes
                </Button>
              )}
            </div>
          </div>

          <div className="p-4">
            {mode === 'form' ? (
              <ContractForm draft={draft} onChange={setDraft} known={known} base={base} problems={problems.fields} attempted={attempted} />
            ) : (
              <div className="flex flex-col gap-2">
                <YamlEditor
                  value={text}
                  onChange={(v) => {
                    setText(v)
                    setYamlError('')
                  }}
                  ariaLabel="Contract manifest"
                />
                {yamlError ? <p className="text-[12px] text-destructive">{yamlError}</p> : <p className="text-[12px] text-faint">YAML or JSON. It is published as written, and Hayat checks it when you publish.</p>}
              </div>
            )}
          </div>

          {mode === 'form' && problems.general.length > 0 && (
            <ul className="mx-4 mb-4 flex flex-col gap-1 rounded-[10px] border border-destructive/30 bg-destructive/5 px-3 py-2 text-[12px] text-destructive">
              {problems.general.map((g) => (
                <li key={g} className="flex items-start gap-2">
                  <TriangleAlert className="mt-0.5 size-3.5 shrink-0" /> {g}
                </li>
              ))}
            </ul>
          )}
        </div>

        <Changes version={version} summary={summary} />
      </div>
    </div>
  )
}
