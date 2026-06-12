import { Link, Outlet, useLocation } from "react-router-dom"
import { cn } from "@/lib/utils"

const NAV_ITEMS = [
  { to: "/", label: "Dashboard" },
  { to: "/artifacts", label: "Artifacts" },
  { to: "/datasets", label: "Datasets" },
  { to: "/indexes", label: "Indexes" },
  { to: "/evaluations", label: "Evaluations" },
]

export function Layout() {
  const location = useLocation()

  const isActive = (to: string) => {
    if (to === "/") return location.pathname === "/"
    return location.pathname.startsWith(to)
  }

  return (
    <div className="flex min-h-svh">
      <aside className="w-56 border-r bg-muted/30 p-4 flex flex-col gap-2">
        <h1 className="text-sm font-semibold px-2 py-1 mb-4">Handbook RAG</h1>
        {NAV_ITEMS.map((item) => (
          <Link
            key={item.to}
            to={item.to}
            className={cn(
              "px-3 py-1.5 text-sm rounded-md transition-colors",
              isActive(item.to)
                ? "bg-accent text-accent-foreground font-medium"
                : "text-muted-foreground hover:text-foreground hover:bg-muted"
            )}
          >
            {item.label}
          </Link>
        ))}
      </aside>
      <main className="flex-1 p-6 min-w-0">
        <Outlet />
      </main>
    </div>
  )
}
