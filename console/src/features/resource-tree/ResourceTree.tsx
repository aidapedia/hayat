import { useEffect, useMemo, useState, type ComponentProps, type ReactNode } from 'react'
import { Link } from 'react-router'
import { ChevronRight, Loader2, MoveRight, Plus } from 'lucide-react'
import type { Node } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { cn } from '@/shared/lib/utils'
import { ContextMenu, ContextMenuContent, ContextMenuItem, ContextMenuLabel, ContextMenuTrigger } from '@/shared/ui/context-menu'
import { NodeIcon } from './NodeIcon'
import { ancestorIds, childrenMap, isContainer, nodeHref, TENANT } from './tree'

/** One tenant's nodes, loaded when the tenant is first opened. */
export type TenantTree = { nodes?: Node[]; loading: boolean; error?: string | null }

export type TreeActions = {
  onCreateGroup: (tenantId: string, parent: Node) => void
  onMove: (tenantId: string, node: Node) => void
}

const rowBase = 'group flex w-full items-center gap-2 rounded-[7px] px-2.5 py-1.5 text-left text-xs'
const rowIdle = 'text-muted-foreground hover:bg-accent hover:text-foreground'

/**
 * A tree row: twisty, icon, name, child count. A row is a link, so a selection is an address.
 *
 * `RowMenu` wraps it in `ContextMenuTrigger asChild`, which hands its right-click
 * and pointer handlers (and a ref) to this component as props — they must reach
 * the `<Link>`, or the trigger never hears the right-click and the browser's own
 * menu opens instead.
 */
function Row({
  to,
  type,
  name,
  count,
  selected,
  expandable,
  expanded,
  busy,
  onToggle,
  onOpen,
  ...triggerProps
}: {
  to: string
  type: string
  name: string
  count?: number
  selected: boolean
  expandable: boolean
  expanded: boolean
  busy?: boolean
  onToggle: () => void
  onOpen: () => void
} & Omit<ComponentProps<typeof Link>, 'to' | 'title' | 'onClick' | 'className'>) {
  return (
    <Link
      {...triggerProps}
      to={to}
      title={name}
      onClick={() => !expanded && expandable && onOpen()}
      className={cn(rowBase, selected ? 'bg-accent font-medium text-foreground' : rowIdle)}
    >
      {/* The twisty always takes the same slot, so a leaf lines its icon up
          with an expandable sibling. Toggling never selects. */}
      <span
        className="flex size-3 shrink-0 items-center justify-center"
        onClick={(e) => {
          if (!expandable) return
          e.preventDefault()
          e.stopPropagation()
          onToggle()
        }}
      >
        {busy ? (
          <Loader2 className="size-3 animate-spin text-faint" />
        ) : (
          expandable && <ChevronRight className={cn('size-3 text-faint transition-transform', expanded && 'rotate-90')} />
        )}
      </span>
      <NodeIcon type={type} className={selected ? 'text-foreground' : 'text-faint group-hover:text-muted-foreground'} />
      <span className="min-w-0 flex-1 truncate">{name}</span>
      {!!count && <span className="shrink-0 text-[10px] font-normal text-faint">{count}</span>}
    </Link>
  )
}

/** Right-click offers what changes the tree's shape: a group inside, or re-filing this node. */
function RowMenu({ node, tenantId, actions, children }: { node?: Node; tenantId: string; actions: TreeActions; children: ReactNode }) {
  if (!node) return children
  const container = isContainer(node.type)
  const movable = node.type !== TENANT
  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
      <ContextMenuContent className="w-52">
        <ContextMenuLabel>{node.name}</ContextMenuLabel>
        {container && (
          <ContextMenuItem onSelect={() => actions.onCreateGroup(tenantId, node)}>
            <Plus /> New group
          </ContextMenuItem>
        )}
        {movable && (
          <ContextMenuItem onSelect={() => actions.onMove(tenantId, node)}>
            <MoveRight /> Move…
          </ContextMenuItem>
        )}
      </ContextMenuContent>
    </ContextMenu>
  )
}

/** The nodes under one tenant, nested, with guide lines down each level. */
function TenantBranch({
  tenantId,
  nodes,
  selectedNode,
  actions,
}: {
  tenantId: string
  nodes: Node[]
  selectedNode?: string
  actions: TreeActions
}) {
  const children = useMemo(() => childrenMap(nodes), [nodes])
  const root = nodes.find((n) => n.type === TENANT)
  // Collapsed rather than expanded ids: a tree that grows while you look at
  // it should show the new node, not hide it.
  const [collapsed, setCollapsed] = useState(() => new Set<string>())

  // Reveal the selection when it changes from outside (a deep link, a new group).
  useEffect(() => {
    if (!selectedNode) return
    const reveal = ancestorIds(nodes, selectedNode)
    setCollapsed((prev) => {
      if (!reveal.some((id) => prev.has(id))) return prev
      const next = new Set(prev)
      for (const id of reveal) next.delete(id)
      return next
    })
  }, [selectedNode, nodes])

  const toggle = (id: string, open?: boolean) =>
    setCollapsed((prev) => {
      const isOpen = !prev.has(id)
      if (open !== undefined && open === isOpen) return prev
      const next = new Set(prev)
      if (isOpen) next.add(id)
      else next.delete(id)
      return next
    })

  const render = (n: Node): ReactNode => {
    const kids = children.get(n.id) ?? []
    const expanded = kids.length > 0 && !collapsed.has(n.id)
    return (
      <div key={n.id}>
        <RowMenu node={n} tenantId={tenantId} actions={actions}>
          <Row
            to={nodeHref(tenantId, n.id)}
            type={n.type}
            name={n.name}
            count={kids.length}
            selected={n.id === selectedNode}
            expandable={kids.length > 0}
            expanded={expanded}
            onToggle={() => toggle(n.id)}
            onOpen={() => toggle(n.id, true)}
          />
        </RowMenu>
        {expanded && <div className="ml-3 flex flex-col gap-0.5 border-l pl-1.5">{kids.map(render)}</div>}
      </div>
    )
  }

  const top = root ? (children.get(root.id) ?? []) : []
  if (top.length === 0) return <p className="py-1.5 pl-7 text-[11px] text-faint">Nothing filed here yet.</p>
  return <>{top.map(render)}</>
}

/**
 * Every tenant, each one a branch that loads the first time it opens. The
 * shape follows tabletsgo's tree: a compact row per node, guide lines down
 * each level, a child count, right-click to add a group or move a node.
 */
export function ResourceTree({
  tenants,
  trees,
  selectedTenant,
  selectedNode,
  onLoad,
  actions,
}: {
  tenants: { id: string; name: string }[]
  trees: Record<string, TenantTree>
  selectedTenant?: string
  /** The selected node; for a tenant's own address, its root node. */
  selectedNode?: string
  onLoad: (tenantId: string) => void
  actions: TreeActions
}) {
  // Tenants start closed (an instance can hold hundreds); the selected one opens.
  const [open, setOpen] = useState(() => new Set<string>(selectedTenant ? [selectedTenant] : []))

  useEffect(() => {
    if (!selectedTenant) return
    setOpen((prev) => (prev.has(selectedTenant) ? prev : new Set(prev).add(selectedTenant)))
  }, [selectedTenant])

  // An open tenant needs its nodes.
  useEffect(() => {
    for (const id of open) if (!trees[id]) onLoad(id)
  }, [open, trees, onLoad])

  const setTenant = (id: string, value: boolean) =>
    setOpen((prev) => {
      if (prev.has(id) === value) return prev
      const next = new Set(prev)
      if (value) next.add(id)
      else next.delete(id)
      return next
    })

  return (
    <div className="flex flex-col gap-0.5">
      {tenants.map((t) => {
        const tree = trees[t.id]
        const root = tree?.nodes?.find((n) => n.type === TENANT)
        const count = tree?.nodes && root ? tree.nodes.filter((n) => n.parentId === root.id).length : undefined
        const expanded = open.has(t.id)
        const selected = selectedTenant === t.id && (!selectedNode || selectedNode === root?.id)
        return (
          <div key={t.id}>
            <RowMenu node={root} tenantId={t.id} actions={actions}>
              <Row
                to={nodeHref(t.id)}
                type={TENANT}
                name={root?.name ?? t.name}
                count={count}
                selected={selected}
                // Unknown until loaded, so a tenant offers to open; once loaded, only if something is inside.
                expandable={count === undefined || count > 0 || !!tree?.error}
                expanded={expanded}
                busy={expanded && tree?.loading && !tree.nodes}
                onToggle={() => setTenant(t.id, !expanded)}
                onOpen={() => setTenant(t.id, true)}
              />
            </RowMenu>
            {expanded && tree && (tree.nodes || tree.error) && (
              <div className="ml-3 flex flex-col gap-0.5 border-l pl-1.5">
                {tree.error ? (
                  <p className="py-1.5 pl-2.5 text-[11px] text-destructive">
                    {tree.error}{' '}
                    <button type="button" className="underline" onClick={() => onLoad(t.id)}>
                      Retry
                    </button>
                  </p>
                ) : (
                  <TenantBranch tenantId={t.id} nodes={tree.nodes!} selectedNode={selectedTenant === t.id ? selectedNode : undefined} actions={actions} />
                )}
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}
