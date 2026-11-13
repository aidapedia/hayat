import { useCallback, useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { FolderTree, RefreshCw, ShieldCheck } from 'lucide-react'
import type { Client } from '@connectrpc/connect'
import { TenantService, type Member } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { TreeAdminService, type Node } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { useMe } from '@/features/auth'
import { errorMessage, useClient } from '@/shared/api/client'
import { usePagedList } from '@/shared/hooks/use-paged-list'
import { useQuery } from '@/shared/hooks/use-query'
import { EmptyState, ErrorState, LoadingState } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { PageHeader } from '@/shared/ui/page/PageHeader'
import { CreateGroupDialog, MoveDialog } from './dialogs'
import { NodePanel } from './NodePanel'
import { ResourceTree, type TenantTree } from './ResourceTree'
import { nodeHref, TENANT } from './tree'

type TreeDialog = { kind: 'group' | 'move'; tenantId: string; node: Node }

/** The whole roster, every page of it: the grant and group pickers choose from all of it. */
async function allMembers(api: Client<typeof TenantService>, tenantId: string) {
  const out: Member[] = []
  let pageToken = ''
  do {
    const res = await api.listMembers({ tenantId, pageSize: 200, pageToken })
    out.push(...res.members)
    pageToken = res.nextPageToken
  } while (pageToken)
  return out
}

/**
 * The Resource Tree: every tenant with the groups and resources filed in it on
 * the left, the selected node's access on the right. The selection is the
 * address (/resource-tree/:tenantId/:nodeId), so a refresh keeps it and a link
 * lands on it. A tenant's nodes load the first time its branch opens.
 */
export default function ResourceTreePage() {
  const { tenantId = '', nodeId } = useParams()
  const { isAdmin } = useMe()
  const navigate = useNavigate()
  const tenantsApi = useClient(TenantService)
  const treeApi = useClient(TreeAdminService)

  const list = usePagedList(
    (token) => tenantsApi.listTenants({ pageSize: 200, pageToken: token }).then((r) => ({ items: r.tenants, next: r.nextPageToken })),
    [tenantsApi],
  )
  const [trees, setTrees] = useState<Record<string, TenantTree>>({})
  // Bumped after a change made from the tree, so the open detail re-reads.
  const [version, setVersion] = useState(0)

  const load = useCallback(
    async (id: string) => {
      setTrees((t) => ({ ...t, [id]: { ...t[id], loading: true, error: null } }))
      try {
        const res = await treeApi.getTenantTree({ tenantId: id })
        setTrees((t) => ({ ...t, [id]: { nodes: res.nodes, loading: false } }))
      } catch (err) {
        setTrees((t) => ({ ...t, [id]: { ...t[id], loading: false, error: errorMessage(err) } }))
      }
    },
    [treeApi],
  )

  const members = useQuery(
    () => (tenantId ? allMembers(tenantsApi, tenantId) : Promise.resolve([])),
    [tenantsApi, tenantId],
  )

  const current = trees[tenantId]
  const nodes = current?.nodes ?? []
  const root = nodes.find((n) => n.type === TENANT)
  const selectedNode = nodeId ?? root?.id
  const tenantName = root?.name ?? ''

  // A deep link may name a tenant past the first page; it still belongs in the tree.
  const tenants = useMemo(() => {
    const items = list.items.map((t) => ({ id: t.id, name: t.name }))
    if (tenantId && root && !items.some((t) => t.id === tenantId)) items.unshift({ id: tenantId, name: root.name })
    return items
  }, [list.items, tenantId, root])

  const refresh = async () => {
    await Promise.all([list.reload(), members.reload(), ...Object.keys(trees).map(load)])
    setVersion((v) => v + 1)
  }

  const [dialog, setDialog] = useState<TreeDialog | null>(null)
  const actions = useMemo(
    () => ({
      onCreateGroup: (t: string, node: Node) => setDialog({ kind: 'group', tenantId: t, node }),
      onMove: (t: string, node: Node) => setDialog({ kind: 'move', tenantId: t, node }),
    }),
    [],
  )

  return (
    <>
      <PageHeader
        title="Resource Tree"
        crumbs={tenantName ? [{ label: tenantName }] : undefined}
        desc={`${isAdmin ? 'Every tenant' : 'Your tenants'}, the groups inside them and the resources services file there, and who has been granted what at each level.`}
        action={
          <Button variant="outline" size="lg" onClick={refresh} disabled={list.loading}>
            <RefreshCw className={list.loading ? 'animate-spin' : undefined} /> Refresh
          </Button>
        }
      />

      {list.error ? (
        <div className="mt-6">
          <ErrorState message={list.error} onRetry={list.reload} />
        </div>
      ) : list.loading && list.items.length === 0 ? (
        <LoadingState />
      ) : tenants.length === 0 ? (
        <div className="mt-6">
          <EmptyState icon={FolderTree} title="No tenants yet">
            The tree starts at a tenant.{' '}
            {isAdmin && (
              <>
                <Link to="/tenants" className="underline">
                  Create one
                </Link>{' '}
                and its groups and resources show up here.
              </>
            )}
          </EmptyState>
        </div>
      ) : (
        <div className="mt-6 grid grid-cols-[320px_minmax(0,1fr)] items-start gap-4 max-[900px]:grid-cols-1">
          {/* Tree */}
          <div className="rounded-xl border bg-card p-3 min-[901px]:sticky min-[901px]:top-4">
            <div className="mb-2 flex items-center gap-2 px-1.5 text-[11px] font-semibold text-muted-foreground">
              <FolderTree className="size-3.5" />
              <span>Tenants</span>
              <span className="ml-auto text-[10px] font-normal text-faint">
                {tenants.length}
                {list.hasMore && '+'} tenant{tenants.length !== 1 ? 's' : ''}
              </span>
            </div>
            <div className="max-h-[calc(100vh-220px)] overflow-y-auto max-[900px]:max-h-none">
              <ResourceTree
                tenants={tenants}
                trees={trees}
                selectedTenant={tenantId || undefined}
                selectedNode={selectedNode}
                onLoad={load}
                actions={actions}
              />
              {list.hasMore && (
                <Button variant="ghost" size="xs" className="mt-1 w-full text-faint" onClick={list.loadMore} disabled={list.loading}>
                  {list.loading ? 'Loading…' : 'Load more tenants'}
                </Button>
              )}
            </div>
            <p className="mt-3 border-t px-1.5 pt-2.5 text-[10px] leading-relaxed text-faint">Right-click a node to add a group or move it.</p>
          </div>

          {/* Detail */}
          <div className="min-w-0 rounded-xl border bg-card">
            {tenantId && current?.error ? (
              <div className="p-4">
                <ErrorState message={current.error} onRetry={() => load(tenantId)} />
              </div>
            ) : tenantId && selectedNode ? (
              <NodePanel
                key={`${selectedNode}:${version}`}
                tenantId={tenantId}
                tenantName={tenantName}
                nodeId={selectedNode}
                nodes={nodes}
                members={members.data ?? []}
                onTreeChange={() => load(tenantId)}
                onClose={() => navigate('/resource-tree')}
              />
            ) : tenantId ? (
              <div className="px-4">
                <LoadingState />
              </div>
            ) : (
              <div className="flex flex-col items-center justify-center gap-3 px-8 py-20 text-center">
                <ShieldCheck className="size-8 text-faint opacity-60" />
                <div>
                  <p className="text-[13px] font-semibold text-muted-foreground">Select a node</p>
                  <p className="mt-1 max-w-[340px] text-[12px] leading-relaxed text-faint">
                    Pick anything in the tree to see who owns it, who has been granted a role on it, and what that role reaches.
                  </p>
                </div>
                <p className="pt-2 text-[11px] text-faint">Access granted on a node applies to everything inside it.</p>
              </div>
            )}
          </div>
        </div>
      )}

      {dialog?.kind === 'group' && (
        <CreateGroupDialog
          parent={dialog.node}
          onClose={() => setDialog(null)}
          onDone={async (g) => {
            const t = dialog.tenantId
            setDialog(null)
            await load(t)
            navigate(nodeHref(t, g.id))
          }}
        />
      )}
      {dialog?.kind === 'move' && (
        <MoveDialog
          node={dialog.node}
          nodes={trees[dialog.tenantId]?.nodes ?? []}
          onClose={() => setDialog(null)}
          onDone={async () => {
            const t = dialog.tenantId
            setDialog(null)
            await load(t)
            setVersion((v) => v + 1)
          }}
        />
      )}
    </>
  )
}
