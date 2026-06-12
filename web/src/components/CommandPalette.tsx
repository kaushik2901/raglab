import { useState, useEffect } from "react"
import { useNavigate } from "react-router-dom"
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command"
import {
  RiDashboardLine,
  RiArchiveLine,
  RiDatabase2Line,
  RiSearch2Line,
  RiLineChartLine,
  RiChat3Line,
  RiAddLine,
} from "@remixicon/react"

const ITEMS = [
  {
    group: "Navigate",
    items: [
      { label: "Dashboard", to: "/", icon: RiDashboardLine, keys: ["d", "h"] },
      { label: "Artifacts", to: "/artifacts", icon: RiArchiveLine, keys: ["a"] },
      { label: "Datasets", to: "/datasets", icon: RiDatabase2Line, keys: ["d"] },
      { label: "Indexes", to: "/indexes", icon: RiSearch2Line, keys: ["i"] },
      { label: "Evaluations", to: "/evaluations", icon: RiLineChartLine, keys: ["e"] },
      { label: "Chat", to: "/chat", icon: RiChat3Line, keys: ["c"] },
    ],
  },
  {
    group: "Actions",
    items: [
      { label: "New Evaluation", to: "/evaluations/new", icon: RiAddLine, keys: ["n", "e"] },
      { label: "New Index", to: "/indexes/new", icon: RiAddLine, keys: ["n", "i"] },
      { label: "New Artifact", to: "/artifacts/new", icon: RiAddLine, keys: ["n", "a"] },
    ],
  },
]

export function CommandPalette() {
  const [open, setOpen] = useState(false)
  const navigate = useNavigate()

  useEffect(() => {
    const down = (e: KeyboardEvent) => {
      if (e.key === "k" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault()
        setOpen((o) => !o)
      }
    }
    document.addEventListener("keydown", down)
    return () => document.removeEventListener("keydown", down)
  }, [])

  return (
    <CommandDialog open={open} onOpenChange={setOpen}>
      <CommandInput placeholder="Type a command or search..." />
      <CommandList>
        <CommandEmpty>No results found.</CommandEmpty>
        {ITEMS.map((group) => (
          <CommandGroup key={group.group} heading={group.group}>
            {group.items.map((item) => (
              <CommandItem
                key={item.to}
                onSelect={() => {
                  navigate(item.to)
                  setOpen(false)
                }}
              >
                <item.icon className="size-4" />
                <span>{item.label}</span>
                <kbd className="ml-auto text-[10px] text-muted-foreground">
                  {item.keys.map((k) => k.toUpperCase()).join(" ")}
                </kbd>
              </CommandItem>
            ))}
          </CommandGroup>
        ))}
      </CommandList>
    </CommandDialog>
  )
}
