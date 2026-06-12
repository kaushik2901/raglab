import { Link, Outlet, useLocation } from "react-router-dom"
import {
  SidebarProvider,
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuItem,
  SidebarMenuButton,
  SidebarFooter,
} from "@/components/ui/sidebar"
import { Separator } from "@/components/ui/separator"
import { RiDashboardLine, RiArchiveLine, RiDatabase2Line, RiSearch2Line, RiLineChartLine, RiChat3Line, RiGitBranchLine } from "@remixicon/react"

interface NavGroup {
  label: string
  items: NavItem[]
}

interface NavItem {
  to: string
  label: string
  icon: React.ComponentType<{ className?: string }>
  matchStartsWith?: boolean
}

const NAV_GROUPS: NavGroup[] = [
  {
    label: "Overview",
    items: [
      { to: "/", label: "Dashboard", icon: RiDashboardLine },
    ],
  },
  {
    label: "Data Pipeline",
    items: [
      { to: "/artifacts", label: "Artifacts", icon: RiArchiveLine },
      { to: "/datasets", label: "Datasets", icon: RiDatabase2Line },
    ],
  },
  {
    label: "Vector Search",
    items: [
      { to: "/indexes", label: "Indexes", icon: RiSearch2Line },
    ],
  },
  {
    label: "Evaluation",
    items: [
      { to: "/evaluations", label: "Evaluations", icon: RiLineChartLine },
    ],
  },
  {
    label: "Tools",
    items: [
      { to: "/chat", label: "Chat", icon: RiChat3Line },
    ],
  },
]

export function Layout() {
  const location = useLocation()

  const isActive = (to: string, matchStartsWith = false) => {
    if (to === "/") return location.pathname === "/"
    if (matchStartsWith) return location.pathname.startsWith(to)
    return location.pathname === to || location.pathname.startsWith(to + "/")
  }

  return (
    <SidebarProvider defaultOpen>
      <div className="flex min-h-svh w-full">
        <Sidebar collapsible="icon">
          <SidebarHeader className="px-3 py-4">
            <Link to="/" className="flex items-center gap-2.5 group-data-[collapsible=icon]:justify-center">
              <div className="flex size-8 items-center justify-center rounded-md bg-primary text-primary-foreground shrink-0">
                <RiGitBranchLine className="size-4.5" />
              </div>
              <span className="text-sm font-semibold tracking-tight group-data-[collapsible=icon]:hidden">
                Handbook RAG
              </span>
            </Link>
          </SidebarHeader>

          <SidebarContent>
            {NAV_GROUPS.map((group, gi) => (
              <SidebarGroup key={group.label}>
                {gi > 0 && <Separator className="mx-3 mb-2 group-data-[collapsible=icon]:hidden" />}
                <SidebarGroupLabel className="group-data-[collapsible=icon]:hidden">
                  {group.label}
                </SidebarGroupLabel>
                <SidebarGroupContent>
                  <SidebarMenu>
                    {group.items.map((item) => {
                      const active = isActive(item.to, item.matchStartsWith ?? true)
                      return (
                        <SidebarMenuItem key={item.to}>
                          <SidebarMenuButton asChild isActive={active} tooltip={item.label}>
                            <Link to={item.to}>
                              <item.icon />
                              <span>{item.label}</span>
                            </Link>
                          </SidebarMenuButton>
                        </SidebarMenuItem>
                      )
                    })}
                  </SidebarMenu>
                </SidebarGroupContent>
              </SidebarGroup>
            ))}
          </SidebarContent>

          <SidebarFooter className="px-3 py-3">
            <div className="flex items-center gap-2 text-xs text-muted-foreground group-data-[collapsible=icon]:hidden">
              <span className="flex size-2 rounded-full bg-success" />
              System Online
            </div>
          </SidebarFooter>
        </Sidebar>

        <main className="flex-1 min-w-0 overflow-auto">
          <div className="mx-auto max-w-7xl p-6 lg:p-8">
            <Outlet />
          </div>
        </main>
      </div>
    </SidebarProvider>
  )
}
