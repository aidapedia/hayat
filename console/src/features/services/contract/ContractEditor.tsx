import { useMemo, useState } from 'react'
import { toast } from 'sonner'
import { Clock, RotateCcw, Sparkles, TriangleAlert } from 'lucide-react'
import { CatalogService, RegistryService, type Contract } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { Button } from '@/shared/ui/button'
import { Input } from '@/shared/ui/input'
import { Section } from '@/shared/ui/page/PageHeader'
import { Tabs, TabsList, TabsTrigger } from '@/shared/ui/tabs'
import { Textarea } from '@/shared/ui/textarea'
import { ContractForm } from './ContractForm'
import { baselineOf, check, draftYaml, emptyDraft, EXAMPLE, manifestYaml, summarize, toDraft, type Baseline, type Draft } from './manifest'
import { SummaryList } from './SummaryList'
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

/**
 * Publishing a contract version: a form for people, YAML for those who keep
 * it in a repo. Both publish the same manifest; the server checks it again.
 */
export function ContractEditor({ serviceId: svc, contracts, onPublished }: { serviceId: string; contracts: Contract[]; onPublished: () => unknown }) {
  const registry = useClient(RegistryService)
  const catalog = useClient(CatalogService)
  const { data: cat } = useQuery(() => catalog.getCatalog({}), [catalog])
  const known = useMemo(() => (cat?.types ?? []).map((t) => ({ key: t.key, service: t.service, description: t.description })), [cat])

  const pending = contracts.find((c) => c.status === 'pending')
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
  const summary = useMemo(() => summarize(draft, base), [draft, base])
  const current = mode === 'form' ? draftYaml(draft) : text
  const dirty = current !== startYaml

  if (pending) {
    return (
      <Section title="Publish a version">
        <div className="flex items-start gap-3 rounded-xl border border-warning/40 bg-warning/5 px-4 py-3.5 text-[13px]">
          <Clock className="mt-0.5 size-4 shrink-0 text-warning" />
          <span>
            Version {pending.version} is waiting for approval. Approve or reject it above before publishing another.
          </span>
        </div>
      </Section>
    )
  }

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
      await registry.publishContract({ serviceId: svc, manifest: current })
      toast.success('Published; it goes live once it is approved')
      await onPublished()
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setPublishing(false)
    }
  }

  const isEmpty = draft.types.length === 0 && draft.permissions.length === 0 && draft.roles.length === 0
  const versionErr = problems.fields.get('version')

  return (
    <Section
      title="Publish a version"
      desc="A new version can only add to the live one. It goes live once every namespace it touches approves it."
      action={
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
      }
    >
      <div className="rounded-xl border">
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
              <Textarea
                value={text}
                onChange={(e) => {
                  setText(e.target.value)
                  setYamlError('')
                }}
                rows={18}
                className="font-mono text-[12px]"
                spellCheck={false}
                aria-label="Contract manifest"
              />
              {yamlError ? <p className="text-[12px] text-destructive">{yamlError}</p> : <p className="text-[12px] text-faint">YAML or JSON. It is published as written, and Hayat checks it when you publish.</p>}
            </div>
          )}
        </div>

        <div className="flex flex-col gap-3 border-t bg-muted/30 px-4 py-3.5">
          {mode === 'form' && (
            <div className="flex flex-col gap-1.5">
              <span className="text-[12px] font-semibold text-muted-foreground">What this version adds</span>
              <SummaryList summary={summary} empty={start.from ? 'Nothing new yet. Add a type, permission or role above.' : 'Nothing yet.'} />
            </div>
          )}
          {mode === 'form' && problems.general.length > 0 && (
            <ul className="flex flex-col gap-1 rounded-[10px] border border-destructive/30 bg-destructive/5 px-3 py-2 text-[12px] text-destructive">
              {problems.general.map((g) => (
                <li key={g} className="flex items-start gap-2">
                  <TriangleAlert className="mt-0.5 size-3.5 shrink-0" /> {g}
                </li>
              ))}
            </ul>
          )}
          <div className="flex items-center justify-end gap-3">
            {!dirty && start.from && <span className="text-[12px] text-faint">Change something to publish a new version.</span>}
            <Button type="button" onClick={publish} disabled={publishing || !dirty || !current.trim()}>
              {publishing ? 'Publishing…' : 'Publish for approval'}
            </Button>
          </div>
        </div>
      </div>
    </Section>
  )
}
