import { useState } from "react"
import { Link, useNavigate } from "react-router-dom"
import { useArtifacts } from "@/hooks/useArtifacts"
import { useCreateIndex } from "@/hooks/useWorkflows"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { RiArrowLeftLine, RiSearch2Line } from "@remixicon/react"

const PROVIDER_PRESETS: Record<string, string> = {
  openai: "text-embedding-3-small",
  gemini: "text-embedding-004",
  openrouter: "openai/text-embedding-3-small",
  lmstudio: "",
}

function StepBadge({ num }: { num: number }) {
  return (
    <div className="flex size-5 items-center justify-center rounded-full bg-primary/10">
      <span className="text-[10px] font-bold text-primary">{num}</span>
    </div>
  )
}

export default function IndexCreate() {
  const navigate = useNavigate()
  const { data: artifacts } = useArtifacts()
  const mutation = useCreateIndex()

  const [inputTag, setInputTag] = useState("")
  const [tag, setTag] = useState("")
  const [parserStrategy] = useState("markdown")
  const [chunkStrategy, setChunkStrategy] = useState("fixed")
  const [chunkSize, setChunkSize] = useState(512)
  const [chunkOverlap, setChunkOverlap] = useState(64)
  const [provider, setProvider] = useState("openai")
  const [model, setModel] = useState("text-embedding-3-small")
  const [batchSize, setBatchSize] = useState(20)
  const [concurrency, setConcurrency] = useState(5)

  const readyArtifacts = (artifacts ?? []).filter((a) => !("pending" in a && a.pending))

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
        chunk_config: chunkStrategy === "recursive"
          ? { max_size: chunkSize, overlap: chunkOverlap }
          : { size: chunkSize, overlap: chunkOverlap },
        embedding_provider: provider,
        embedding_model: model,
        batch_size: batchSize,
        index_concurrency: concurrency,
        doc_timeout: "30m",
      },
      { onSuccess: () => navigate("/indexes") }
    )
  }

  return (
    <div className="max-w-2xl space-y-8">
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="icon" asChild className="size-8">
          <Link to="/indexes">
            <RiArrowLeftLine className="size-4" />
          </Link>
        </Button>
        <div>
          <h1 className="text-2xl font-bold tracking-tight">New Index</h1>
          <p className="text-sm text-muted-foreground mt-1">
            Parse, chunk, and embed preprocessed documents into a Qdrant collection.
          </p>
        </div>
      </div>

      <Separator />

      <form onSubmit={handleSubmit} className="space-y-6">
        <Card>
          <CardHeader>
            <div className="flex items-center gap-2 mb-1">
              <StepBadge num={1} />
              <CardTitle className="text-base">Source Artifact</CardTitle>
            </div>
            <CardDescription>Select a preprocessed artifact to index.</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-2">
              <Label htmlFor="input_tag">Artifact</Label>
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
            <div className="flex items-center gap-2 mb-1">
              <StepBadge num={2} />
              <CardTitle className="text-base">Identity & Parsing</CardTitle>
            </div>
            <CardDescription>Name your index and configure chunking strategy.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="tag">Collection Tag</Label>
              <Input id="tag" value={tag} onChange={(e) => setTag(e.target.value)} placeholder="e.g. handbook-index" required />
            </div>
            <div className="space-y-2">
              <Label>Parser Strategy</Label>
              <Input value={parserStrategy} disabled className="text-muted-foreground" />
            </div>
            <div className="space-y-2">
              <Label>Chunk Strategy</Label>
              <Select value={chunkStrategy} onValueChange={setChunkStrategy}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="fixed">Fixed (word-window splitter)</SelectItem>
                  <SelectItem value="recursive">Recursive (heading-aware splitter)</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="chunk_size">Chunk Size</Label>
                <Input id="chunk_size" type="number" value={chunkSize} onChange={(e) => setChunkSize(Number(e.target.value))} min={64} max={4096} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="chunk_overlap">Overlap</Label>
                <Input id="chunk_overlap" type="number" value={chunkOverlap} onChange={(e) => setChunkOverlap(Number(e.target.value))} min={0} max={1024} />
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <div className="flex items-center gap-2 mb-1">
              <StepBadge num={3} />
              <CardTitle className="text-base">Embedding</CardTitle>
            </div>
            <CardDescription>Choose the embedding provider and model.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="provider">Provider</Label>
                <Select value={provider} onValueChange={handleProviderChange}>
                  <SelectTrigger id="provider">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="openai">OpenAI</SelectItem>
                    <SelectItem value="gemini">Gemini</SelectItem>
                    <SelectItem value="openrouter">OpenRouter</SelectItem>
                    <SelectItem value="lmstudio">LM Studio</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="model">Model</Label>
                <Input id="model" value={model} onChange={(e) => setModel(e.target.value)} required />
              </div>
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="batch_size">Batch Size</Label>
                <Input id="batch_size" type="number" value={batchSize} onChange={(e) => setBatchSize(Number(e.target.value))} min={1} max={100} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="concurrency">Concurrency</Label>
                <Input id="concurrency" type="number" value={concurrency} onChange={(e) => setConcurrency(Number(e.target.value))} min={1} max={20} />
              </div>
            </div>
          </CardContent>
        </Card>

        <div className="flex gap-3 pt-2">
          <Button type="submit" disabled={mutation.isPending} size="lg">
            <RiSearch2Line className="size-4" />
            {mutation.isPending ? "Indexing..." : "Create Index"}
          </Button>
          <Button type="button" variant="outline" size="lg" asChild>
            <Link to="/indexes">Cancel</Link>
          </Button>
        </div>
      </form>
    </div>
  )
}
