import { useState, type ComponentType, type MouseEvent } from 'react'
import { Link, Outlet, useLocation, useNavigate } from 'react-router'
import {
  AppWindow,
  Boxes,
  Building2,
  KeyRound,
  LayoutDashboard,
  LogOut,
  Monitor,
  Moon,
  MoreHorizontal,
  PanelLeft,
  ScrollText,
  ShieldCheck,
  ShieldHalf,
  FolderTree,
  Sun,
  UserRound,
  Users,
} from 'lucide-react'
import { useAuth, useMe } from '@/features/auth'
import { useTheme, type Theme } from '@/shared/lib/theme'
import { PersonAvatar } from '@/shared/ui/bits'
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/shared/ui/breadcrumb'
import { Button } from '@/shared/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/shared/ui/dropdown-menu'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarSeparator,
  SidebarTrigger,
  useSidebar,
} from '@/shared/ui/sidebar'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/shared/ui/tooltip'
import { PageTrailProvider, usePageCrumbs } from '@/shared/ui/page/PageTrail'

type NavEntry = { label: string; Icon: ComponentType<{ className?: string }>; path: string; exact?: boolean }
type NavModel = { root: { label: string; path: string }; pinned: NavEntry[]; groups: { group: string; items: NavEntry[] }[] }

/**
 * Sidebar model: a pinned row that answers "where am I", then titled groups.
 * Every group earns its header. A row is active for its path and everything
 * under it, so /users/42 still reads as "Users". The Account group isn't
 * listed: it opens from the account menu, next to sign-out, but stays in the
 * model so the trail can name it.
 */
const ADMIN_NAV: NavModel = {
  root: { label: 'Administration', path: '/' },
  pinned: [{ label: 'Overview', Icon: LayoutDashboard, path: '/', exact: true }],
  groups: [
    {
      group: 'Directory',
      items: [
        { label: 'Users', Icon: Users, path: '/users' },
        { label: 'Tenants', Icon: Building2, path: '/tenants' },
      ],
    },
    {
      group: 'Access',
      items: [
        { label: 'Resource Tree', Icon: FolderTree, path: '/resource-tree' },
        { label: 'Roles', Icon: ShieldHalf, path: '/roles' },
      ],
    },
    {
      group: 'Applications',
      items: [
        { label: 'Services', Icon: Boxes, path: '/services' },
        { label: 'OAuth clients', Icon: AppWindow, path: '/clients' },
      ],
    },
    {
      group: 'Security',
      items: [
        { label: 'Signing keys', Icon: KeyRound, path: '/keys' },
        { label: 'Audit log', Icon: ScrollText, path: '/audit' },
      ],
    },
    { group: 'Account', items: [{ label: 'Account', Icon: UserRound, path: '/account' }] },
  ],
}

// Someone who belongs to a tenant manages it here: its members and its
// Resource Tree. What they may change is decided by the server, per action.
const MEMBER_NAV: NavModel = {
  root: { label: 'Tenants', path: '/tenants' },
  pinned: [{ label: 'Tenants', Icon: Building2, path: '/tenants' }],
  groups: [
    { group: 'Access', items: [{ label: 'Resource Tree', Icon: FolderTree, path: '/resource-tree' }] },
    { group: 'Account', items: [{ label: 'Account', Icon: UserRound, path: '/account' }] },
  ],
}

// Someone who isn't an instance admin and belongs to no tenant has one thing to manage here: themselves.
const USER_NAV: NavModel = {
  root: { label: 'Account', path: '/account' },
  pinned: [{ label: 'Account', Icon: UserRound, path: '/account' }],
  groups: [],
}

const isActive = (pathname: string, { path, exact }: { path: string; exact?: boolean }) =>
  exact ? pathname === path : pathname === path || pathname.startsWith(`${path}/`)

function PageTrail({ nav }: { nav: NavModel }) {
  const { pathname } = useLocation()
  const crumbs = usePageCrumbs()
  const section = [...nav.pinned, ...nav.groups.flatMap((g) => g.items)].find((item) => isActive(pathname, item))
  const places = section && section.path !== nav.root.path ? [nav.root, section] : [nav.root]
  const all = [...places.map((p) => ({ label: p.label, to: p.path })), ...crumbs]

  return (
    <Breadcrumb className="mb-3">
      <BreadcrumbList className="text-[12px]">
        {all.map((c, i) => {
          const last = i === all.length - 1
          return (
            <span key={`${c.label}-${i}`} className="contents">
              <BreadcrumbItem>
                {last || !c.to ? (
                  <BreadcrumbPage className="text-[12px]">{c.label}</BreadcrumbPage>
                ) : (
                  <BreadcrumbLink asChild>
                    <Link to={c.to}>{c.label}</Link>
                  </BreadcrumbLink>
                )}
              </BreadcrumbItem>
              {!last && <BreadcrumbSeparator />}
            </span>
          )
        })}
      </BreadcrumbList>
    </Breadcrumb>
  )
}

function Brand() {
  const { state, toggleSidebar, isMobile } = useSidebar()
  const collapsed = state === 'collapsed' && !isMobile
  if (collapsed) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            onClick={toggleSidebar}
            aria-label="Expand sidebar"
            className="group relative flex size-10 items-center justify-center rounded-[10px] text-muted-foreground hover:text-foreground"
          >
            <img src="/hayat.png" alt="" className="size-9 rounded-[9px] transition-opacity group-hover:opacity-0" />
            <PanelLeft className="absolute size-4 opacity-0 transition-opacity group-hover:opacity-100" />
          </button>
        </TooltipTrigger>
        <TooltipContent side="right">Expand sidebar</TooltipContent>
      </Tooltip>
    )
  }
  return (
    <div className="flex items-center gap-2.5">
      <img src="/hayat.png" alt="" className="size-8 shrink-0 rounded-[8px]" />
      <div className="min-w-0 flex-1">
        <div className="truncate text-[15px] font-bold tracking-[-0.3px]">
          Hayat
        </div>
        <div className="truncate text-[10px] text-faint">Identity and access</div>
      </div>
      {!isMobile && (
        <Button variant="ghost" size="icon" onClick={toggleSidebar} aria-label="Collapse sidebar">
          <PanelLeft className="size-4" />
        </Button>
      )}
    </div>
  )
}

function AdminBadge() {
  const { state, isMobile } = useSidebar()
  if (state === 'collapsed' && !isMobile) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="flex size-10 items-center justify-center">
            <span className="flex size-9 items-center justify-center rounded-[11px] bg-primary text-primary-foreground">
              <ShieldCheck className="size-4" />
            </span>
          </span>
        </TooltipTrigger>
        <TooltipContent side="right">Administration</TooltipContent>
      </Tooltip>
    )
  }
  return (
    <div className="flex items-center gap-2.5 rounded-[10px] border bg-muted px-2.5 py-2">
      <span className="flex size-7 shrink-0 items-center justify-center rounded-[8px] bg-primary text-primary-foreground">
        <ShieldCheck className="size-3.5" />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[12px] font-semibold">Administration</span>
        <span className="block text-[10px] text-faint">Instance admin</span>
      </span>
    </div>
  )
}

function NavRows({ items }: { items: NavEntry[] }) {
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const { setOpenMobile } = useSidebar()
  // Rows are real links, so a modified click opens a new tab; only a plain
  // click is taken over for client-side routing.
  const go = (e: MouseEvent, path: string) => {
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || e.button > 0) return
    e.preventDefault()
    setOpenMobile(false)
    navigate(path)
  }
  return (
    <SidebarMenu className="gap-0.5">
      {items.map((item) => {
        const active = isActive(pathname, item)
        return (
          <SidebarMenuItem key={item.path}>
            <SidebarMenuButton asChild isActive={active} tooltip={item.label} className={active ? 'font-semibold' : 'text-muted-foreground'}>
              <a href={item.path} onClick={(e) => go(e, item.path)}>
                <item.Icon className={active ? 'text-primary' : ''} />
                <span>{item.label}</span>
              </a>
            </SidebarMenuButton>
          </SidebarMenuItem>
        )
      })}
    </SidebarMenu>
  )
}

function AccountMenu() {
  const { me } = useMe()
  const { signOut } = useAuth()
  const { theme, setTheme } = useTheme()
  const navigate = useNavigate()
  const { state, isMobile, setOpenMobile } = useSidebar()
  const collapsed = state === 'collapsed' && !isMobile
  const user = me.user!

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        {collapsed ? (
          <button aria-label="Account" className="flex size-10 items-center justify-center rounded-[10px] hover:bg-sidebar-accent">
            <PersonAvatar name={user.name} email={user.email} size={30} />
          </button>
        ) : (
          <button
            aria-label="Account"
            className="flex w-full items-center gap-2.5 rounded-xl px-2 py-2 text-left transition-colors hover:bg-sidebar-accent data-[state=open]:bg-sidebar-accent"
          >
            <PersonAvatar name={user.name} email={user.email} size={30} />
            <span className="min-w-0 flex-1 truncate text-[13px] font-semibold">{user.name || user.email}</span>
            <MoreHorizontal className="size-4 shrink-0 text-faint" />
          </button>
        )}
      </DropdownMenuTrigger>
      <DropdownMenuContent side={collapsed ? 'right' : 'top'} align="start" className="w-58">
        <DropdownMenuLabel className="font-normal">
          <div className="truncate text-[13px] font-semibold">{user.name || 'Your account'}</div>
          <div className="truncate text-[12px] text-muted-foreground">{user.email}</div>
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          className="text-[13px]"
          onSelect={() => {
            setOpenMobile(false)
            navigate('/account')
          }}
        >
          <UserRound /> Account settings
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuLabel className="text-[10px] font-bold tracking-wide text-faint uppercase">Theme</DropdownMenuLabel>
        <DropdownMenuRadioGroup value={theme} onValueChange={(v) => setTheme(v as Theme)}>
          <DropdownMenuRadioItem value="light" className="text-[13px]">
            <Sun /> Light
          </DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="dark" className="text-[13px]">
            <Moon /> Dark
          </DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="system" className="text-[13px]">
            <Monitor /> System
          </DropdownMenuRadioItem>
        </DropdownMenuRadioGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem className="text-[13px]" onSelect={() => void signOut()}>
          <LogOut /> Sign out
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

const COLLAPSE_KEY = 'hayat:sidebar-open'
const loadOpen = () => {
  try {
    return localStorage.getItem(COLLAPSE_KEY) !== '0'
  } catch {
    return true
  }
}

/** Home shell: sidebar + the active page. */
export default function HomeLayout() {
  const { isAdmin, me } = useMe()
  const nav = isAdmin ? ADMIN_NAV : me.tenants.length > 0 ? MEMBER_NAV : USER_NAV
  const listed = nav.groups.filter((g) => g.group !== 'Account')
  const [open, setOpen] = useState(loadOpen)

  return (
    <SidebarProvider
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        try {
          localStorage.setItem(COLLAPSE_KEY, next ? '1' : '0')
        } catch {
          /* layout preference only */
        }
      }}
      className="h-full"
    >
      <Sidebar collapsible="icon">
        <SidebarHeader className="border-b px-4 py-4 group-data-[collapsible=icon]:px-2">
          <Brand />
        </SidebarHeader>
        {isAdmin && (
          <div className="px-3 pt-3 group-data-[collapsible=icon]:px-2">
            <AdminBadge />
          </div>
        )}
        <SidebarContent className="gap-0 px-1 pt-1 group-data-[collapsible=icon]:px-0">
          <SidebarGroup>
            <NavRows items={nav.pinned} />
          </SidebarGroup>
          {listed.map((g) => (
            <SidebarGroup key={g.group} className="pt-0">
              {/* Collapsed, the label hides by pulling itself up by its own
                  height — so that pull must match `h-6`. The stock `-mt-8` is
                  sized for an `h-8` label and would eat 8px more: the previous
                  group's bottom padding, leaving the rule flush against its
                  last row. With it matched, the rule gets 8px either side. */}
              <SidebarGroupLabel className="h-6 px-2.5 text-[10px] font-bold tracking-wide text-faint uppercase group-data-[collapsible=icon]:-mt-6">{g.group}</SidebarGroupLabel>
              <SidebarSeparator className="mx-0 mb-2 hidden group-data-[collapsible=icon]:block" />
              <NavRows items={g.items} />
            </SidebarGroup>
          ))}
        </SidebarContent>
        <SidebarFooter className="border-t px-3 py-3 group-data-[collapsible=icon]:px-2">
          <AccountMenu />
        </SidebarFooter>
      </Sidebar>

      <SidebarInset className="min-w-0">
        <div className="flex items-center gap-3 border-b bg-sidebar px-4 py-3 md:hidden">
          <SidebarTrigger className="size-9" />
          <span className="text-[15px] font-bold tracking-[-0.3px]">
            Hayat
          </span>
        </div>
        {/* The one content container: every page gets the same width and gutters. */}
        <main className="min-h-0 flex-1 overflow-y-auto px-8 py-10 max-[600px]:px-4 max-[600px]:py-7">
          <div className="mx-auto w-full max-w-[1280px]">
            <PageTrailProvider>
              <PageTrail nav={nav} />
              <Outlet />
            </PageTrailProvider>
          </div>
        </main>
      </SidebarInset>
    </SidebarProvider>
  )
}
