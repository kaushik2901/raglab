import { useState, type KeyboardEvent } from "react"
import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
import { RiCloseLine } from "@remixicon/react"

interface ChipInputProps {
  value: string[]
  onChange: (chips: string[]) => void
  placeholder?: string
}

export function ChipInput({ value, onChange, placeholder = "Type and press Enter..." }: ChipInputProps) {
  const [input, setInput] = useState("")

  const addChip = () => {
    const trimmed = input.trim()
    if (trimmed && !value.includes(trimmed)) {
      onChange([...value, trimmed])
    }
    setInput("")
  }

  const removeChip = (chip: string) => {
    onChange(value.filter((c) => c !== chip))
  }

  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      e.preventDefault()
      addChip()
    } else if (e.key === "Backspace" && input === "" && value.length > 0) {
      removeChip(value[value.length - 1])
    }
  }

  return (
    <div className="flex flex-wrap items-center gap-1.5 rounded-md border bg-background px-2 py-1.5 min-h-9">
      {value.map((chip) => (
        <Badge key={chip} variant="secondary" className="gap-1 pr-1">
          {chip}
          <button
            type="button"
            onClick={() => removeChip(chip)}
            className="ml-0.5 rounded-full hover:bg-muted-foreground/20 p-0.5 cursor-pointer"
            aria-label={`Remove ${chip}`}
          >
            <RiCloseLine className="size-3" />
          </button>
        </Badge>
      ))}
      <Input
        value={input}
        onChange={(e) => setInput(e.target.value)}
        onKeyDown={handleKeyDown}
        onBlur={addChip}
        placeholder={value.length === 0 ? placeholder : ""}
        className="flex-1 min-w-[120px] border-0 bg-transparent px-1 py-0 h-auto text-sm focus-visible:ring-0 focus-visible:ring-offset-0 shadow-none"
      />
    </div>
  )
}
