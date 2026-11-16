import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router'
import { UserPlus, Users } from 'lucide-react'
import { UserService } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { useClient } from '@/shared/api/client'
import { useServerPagination } from '@/shared/ui/table/usePagination'
import { formatDate } from '@/shared/lib/datetime'
import { EmptyState, ErrorState, LoadingState, PersonRow, Pill, SearchInput, StatusBadge } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { PageHeader } from '@/shared/ui/page/PageHeader'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'
import { Pagination } from '@/shared/ui/table/Pagination'
import { InviteUserDialog } from './InviteUserDialog'

function useDebounced<T>(value: T, ms = 250) {
  const [v, setV] = useState(value)
  useEffect(() => {
    const t = setTimeout(() => setV(value), ms)
    return () => clearTimeout(t)
  }, [value, ms])
  return v
}

export default function UsersPage() {
  const users = useClient(UserService)
  const navigate = useNavigate()
  const [query, setQuery] = useState('')
  const [inviting, setInviting] = useState(false)
  const q = useDebounced(query)

  const list = useServerPagination(
    async (token, pageSize) => {
      const res = await users.listUsers({ query: q, pageSize, pageToken: token })
      return { items: res.users, next: res.nextPageToken }
    },
    [users, q],
  )

  return (
    <>
      <PageHeader
        title="Users"
        desc="Everyone with a Hayat account. One account signs in to every application that uses Hayat."
        action={
          <Button size="lg" onClick={() => setInviting(true)}>
            <UserPlus /> Invite user
          </Button>
        }
      />
      <div className="mt-7 flex flex-col gap-4">
        <SearchInput placeholder="Search by name or email" value={query} onChange={(e) => setQuery(e.target.value)} className="max-w-sm" />
        {list.error ? (
          <ErrorState message={list.error} onRetry={list.reload} />
        ) : list.loading && list.items.length === 0 ? (
          <LoadingState />
        ) : list.items.length === 0 ? (
          <EmptyState icon={Users} title={q ? 'No one matches that search' : 'No users yet'}>
            {!q && 'Invite someone, or let people sign up from the Hayat sign-in page.'}
          </EmptyState>
        ) : (
          <div className="overflow-hidden rounded-xl border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="text-[11px]">Person</TableHead>
                  <TableHead className="text-[11px]">Status</TableHead>
                  <TableHead className="text-[11px]">Email</TableHead>
                  <TableHead className="text-[11px]">Joined</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {list.items.map((u) => (
                  <TableRow key={u.id} className="cursor-pointer" onClick={() => navigate(`/users/${u.id}`)}>
                    <TableCell>
                      <PersonRow
                        name={u.name}
                        email={u.email}
                        trailing={u.isAdmin ? <Pill className="border-primary/30 text-primary">Admin</Pill> : undefined}
                      />
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={u.status} />
                    </TableCell>
                    <TableCell className="text-[12px] text-muted-foreground">{u.emailVerified ? 'Verified' : 'Not verified'}</TableCell>
                    <TableCell className="text-[12px] text-muted-foreground">{formatDate(u.createdAt)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            <Pagination {...list.pager} className="border-t" />
          </div>
        )}
      </div>
      <InviteUserDialog open={inviting} onOpenChange={setInviting} />
    </>
  )
}
