import { useState, useRef, type DragEvent } from "react"
import { Progress } from "@/components/ui/progress"

interface FileUploadProps {
  onUpload: (file: File) => void
  uploading: boolean
  progress?: number
  accept?: string
}

export function FileUpload({ onUpload, uploading, progress, accept = ".jsonl" }: FileUploadProps) {
  const [dragOver, setDragOver] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  const handleDrop = (e: DragEvent) => {
    e.preventDefault()
    setDragOver(false)
    const file = e.dataTransfer.files[0]
    if (file) onUpload(file)
  }

  const handleChange = () => {
    const file = inputRef.current?.files?.[0]
    if (file) onUpload(file)
  }

  return (
    <div className="space-y-3">
      <div
        onDragOver={(e) => { e.preventDefault(); setDragOver(true) }}
        onDragLeave={() => setDragOver(false)}
        onDrop={handleDrop}
        onClick={() => inputRef.current?.click()}
        className={`border-2 border-dashed rounded-lg p-8 text-center cursor-pointer transition-colors ${
          dragOver
            ? "border-primary bg-primary/5"
            : "border-muted-foreground/25 hover:border-muted-foreground/50"
        }`}
      >
        <input
          ref={inputRef}
          type="file"
          accept={accept}
          onChange={handleChange}
          className="hidden"
        />
        <p className="text-sm text-muted-foreground">
          {uploading
            ? "Uploading..."
            : `Drop a ${accept} file here, or click to browse`}
        </p>
      </div>
      {uploading && progress != null && (
        <Progress value={progress} className="h-2" />
      )}
    </div>
  )
}
