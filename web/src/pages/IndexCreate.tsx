import { useState } from "react"
import { Link, useNavigate } from "react-router-dom"
import { useArtifacts } from "@/hooks/useArtifacts"
import { useCreateIndex } from "@/hooks/useWorkflows"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { RiArrowLeftLine } from "@remixicon/react"

const PROVIDER_PRESETS: Record<string, string> = {
  openai: "text-embedding-3-small",
  gemini: "text-embedding-004",
  openrouter: "openai/text-embedding-3-small",
  lmstudio: "",
}

export default function IndexCreate() {
  const navigate = useNavigate()
  const { data: artifacts } = useArtifacts()
  const mutation = useCreateIndex()

  const [inputTag, setInputTag] = useState("")
  const [tag, setTag] = useState("")
  const [parserStrategy] = useState("markdown")
  const [chunkStrategy] = useState("fixed")
  const [chunkSize, setChunkSize] = useState(512)
  const [chunkOverlap, setChunkOverlap] = useState(64)
  const [provider, setProvider] = useState("openai")
  const [model, setModel] = useState("text-embedding-3-small")
  const [batchSize, setBatchSize] = useState(20)
  const [concurrency, setConcurrency] = useState(5)
  const [docTimeout, setDocTimeout] = useState("30m")

  const handleProviderChange = (p: string) => {
    setProvider(p)
    setModel(PROVIDER_PRESETS[p] ?? "")
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    mutation.mutate(
      {
        input_tag: inputTag,
        tag,
        parser_strategy: parserStrategy,
        chunk_strategy: chunkStrategy,
        chunk_config: { size: chunkSize, overlap: chunkOverlap },
        embedding_provider: provider,
        embedding_model: model,
        batch_size: batchSize,
        index_concurrency: concurrency,
        doc_timeout: docTimeout,
      },
      { onSuccess: () => navigate("/indexes") }
    )
  }

  // Filter out pending artifacts
  const readyArtifacts = (artifacts ?? []).filter((a) => !("pending" in a && a.pending))

  return (
    <div className="mx-auto max-w-xl space-y-6">
      <div className="flex items-center gap-2">
        <Button variant="ghost" size="icon" asChild className="size-8">
          <Link to="/indexes">
            <RiArrowLeftLine className="size-4" />
          </Link>
        </Button>
        <h2 className="text-lg font-semibold">New Index</h2>
      </div>

      <form onSubmit={handleSubmit} className="space-y-6">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Source</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-1.5">
              <Label htmlFor="input_tag">Artifact *</Label>
              <Select value={inputTag} onValueChange={setInputTag}>
                <SelectTrigger id="input_tag">
                  <SelectValue placeholder="Select an artifact..." />
                </SelectTrigger>
                <SelectContent>
                  {readyArtifacts.map((a) => (
                    <SelectItem key={a.tag} value={a.tag}>
                      {a.tag} ({a.file_count?.toLocaleString() ?? "?"} files)
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Identity</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-1.5">
              <Label htmlFor="tag">Tag *</Label>
              <Input
                id="tag"
                value={tag}
                onChange={(e) => setTag(e.target.value)}
                placeholder="e.g. handbook-index"
                required
              />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Parsing</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-1.5">
              <Label>Parser Strategy</Label>
              <Input value={parserStrategy} disabled />
            </div>
            <div className="space-y-1.5">
              <Label>Chunk Strategy</Label>
              <Input value={chunkStrategy} disabled />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <Label htmlFor="chunk_size">Size</Label>
                <Input
                  id="chunk_size"
                  type="number"
                  value={chunkSize}
                  onChange={(e) => setChunkSize(Number(e.target.value))}
                  min={64}
                  max={4096}
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="chunk_overlap">Overlap</Label>
                <Input
                  id="chunk_overlap"
                  type="number"
                  value={chunkOverlap}
                  onChange={(e) => setChunkOverlap(Number(e.target.value))}
                  min={0}
                  max={1024}
                />
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Embedding</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="provider">Provider *</Label>
              <Select value={provider} onValueChange={handleProviderChange}>
                <SelectTrigger id="provider">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="openai">openai</SelectItem>
                  <SelectItem value="gemini">gemini</SelectItem>
                  <SelectItem value="openrouter">openrouter</SelectItem>
                  <SelectItem value="lmstudio">lmstudio</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="model">Model *</Label>
              <Input
                id="model"
                value={model}
                onChange={(e) => setModel(e.target.value)}
                required
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="batch_size">Batch Size</Label>
              <Input
                id="batch_size"
                type="number"
                value={batchSize}
                onChange={(e) => setBatchSize(Number(e.target.value))}
                min={1}
                max={100}
              />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Performance</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="concurrency">Index Concurrency</Label>
              <Input
                id="concurrency"
                type="number"
                value={concurrency}
                onChange={(e) => setConcurrency(Number(e.target.value))}
                min={1}
                max={20}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="doc_timeout">Doc Timeout</Label>
              <Input
                id="doc_timeout"
                value={docTimeout}
                onChange={(e) => setDocTimeout(e.target.value)}
                placeholder="30m"
              />
            </div>
          </CardContent>
        </Card>

        <div className="flex gap-2">
          <Button type="submit" disabled={mutation.isPending}>
            {mutation.isPending ? "Creating..." : "Create"}
          </Button>
          <Button type="button" variant="outline" asChild>
            <Link to="/indexes">Cancel</Link>
          </Button>
        </div>
      </form>
    </div>
  )
}
