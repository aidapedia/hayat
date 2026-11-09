import { Box, Building2, Users } from 'lucide-react'
import { cn } from '@/shared/lib/utils'
import { GROUP, TENANT } from './tree'

/**
 * One icon per kind of node, so a node looks the same in the tree, the detail
 * header and the grant list. A group draws as people rather than a folder: it
 * is a container and a roster, and the roster is what a grant is made to.
 */
export function NodeIcon({ type, className }: { type: string; className?: string }) {
  const Icon = type === TENANT ? Building2 : type === GROUP ? Users : Box
  return <Icon className={cn('size-3.5 shrink-0', className)} />
}
