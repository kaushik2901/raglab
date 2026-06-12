import { useState } from "react"
import { Link, useNavigate } from "react-router-dom"
import { useIndexes } from "@/hooks/useIndexes"
import { useDatasets } from "@/hooks/useDatasets"
import { useCreateEval } from "@/hooks/useWorkflows"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Badge } from "@/components/ui/badge"
import { RiArrowLeftLine } from "@remixicon/react"

const K_OPTIONS = [1, 3, 5, 10, 20]

const LLM_PRESETS: Record<string, string> = {
  openai: "gpt-4o-mini",
  gemini: "gemini-2.0-flash",
  openrouter: "openai/gpt-4o-mini",
  lmstudio: "",
}

const EMBED_PRESETS: Record<string, string> = {
  openai: "text-embedding-3-small",
  gemini: "text-embedding-004",
  openrouter: "openai/text-embedding-3-small",
  lmstudio: "",
}

export default function EvalCreate() {
  const navigate = useNavigate()
  const { data: indexes } = useIndexes()
  const { data: datasets } = useDatasets()
  const mutation = useCreateEval()

  const [indexTag, setIndexTag] = useState("")
  const [datasetPath, setDatasetPath] = useState("")
  const [tag, setTag] = useState("")
  const [queryStrategy] = useState("naive-search")
  const [ks, setKs] = useState<number[]>([1, 3, 5])
  const [llmProvider, setLlmProvider] = useState("openai")
  const [llmModel, setLlmModel] = useState("gpt-4o-mini")
  const [embedProvider, setEmbedProvider] = useState("openai")
  const [embedModel, setEmbedModel] = useState("text-embedding-3-small")
  const [judgeProvider, setJudgeProvider] = useState("openai")
  const [judgeModel, setJudgeModel] = useState("gpt-4o-mini")
  const [batchSize, setBatchSize] = useState(20)
  const [workers, setWorkers] = useState(5)

  const toggleK = (k: number) => {
    setKs((prev) => (prev.includes(k) ? prev.filter((v) => v !== k) : [...prev, k].sort((a, b) => a - b)))
  }

  const readyIndexes = (indexes ?? []).filter((i) => !("pending" in i && i.pending))

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    mutation.mutate(
      {
        index_tag: indexTag,
        tag,
        query_strategy: queryStrategy,
        dataset_path: datasetPath,
        ks,
        llm_provider: llmProvider,
        llm_model: llmModel,
        embedding_provider: embedProvider,
        embedding_model: embedModel,
        judge_provider: judgeProvider,
        judge_model: judgeModel,
        batch_size: batchSize,
        workers,
      },
      { onSuccess: () => navigate("/evaluations") }
    )
  }

  return (
    <div className="mx-auto max-w-xl space-y-6">
      <div className="flex items-center gap-2">
        <Button variant="ghost" size="icon" asChild className="size-8">
          <Link to="/evaluations">
            <RiArrowLeftLine className="size-4" />
          </Link>
        </Button>
        <h2 className="text-lg font-semibold">New Evaluation</h2>
      </div>

      <form onSubmit={handleSubmit} className="space-y-6">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Source</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="index_tag">Index *</Label>
              <Select value={indexTag} onValueChange={setIndexTag}>
                <SelectTrigger id="index_tag">
                  <SelectValue placeholder="Select an index..." />
                </SelectTrigger>
                <SelectContent>
                  {readyIndexes.map((idx) => (
                    <SelectItem key={idx.name} value={idx.name}>
                      {idx.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="dataset">Dataset *</Label>
              <Select value={datasetPath} onValueChange={setDatasetPath}>
                <SelectTrigger id="dataset">
                  <SelectValue placeholder="Select a dataset..." />
                </SelectTrigger>
                <SelectContent>
                  {(datasets ?? []).map((ds) => (
                    <SelectItem key={ds.name} value={ds.name}>
                      {ds.name} ({ds.question_count} questions)
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Identity & Strategy</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="tag">Tag *</Label>
              <Input id="tag" value={tag} onChange={(e) => setTag(e.target.value)} placeholder="e.g. eval-v1" required />
            </div>
            <div className="space-y-1.5">
              <Label>Query Strategy</Label>
              <Input value={queryStrategy} disabled />
            </div>
            <div className="space-y-1.5">
              <Label>K Values</Label>
              <div className="flex flex-wrap gap-1.5">
                {K_OPTIONS.map((k) => {
                  const active = ks.includes(k)
                  return (
                    <Badge
                      key={k}
                      variant={active ? "default" : "outline"}
                      className="cursor-pointer select-none"
                      onClick={() => toggleK(k)}
                    >
                      {k}
                    </Badge>
                  )
                })}
              </div>
            </div>
          </CardContent>
        </Card>

        {(["llm", "embed", "judge"] as const).map((role) => {
          const provider = role === "llm" ? llmProvider : role === "embed" ? embedProvider : judgeProvider
          const model = role === "llm" ? llmModel : role === "embed" ? embedModel : judgeModel
          const setProvider = role === "llm" ? setLlmProvider : role === "embed" ? setEmbedProvider : setJudgeProvider
          const setModel = role === "llm" ? setLlmModel : role === "embed" ? setEmbedModel : setJudgeModel
          const presets = role === "embed" ? EMBED_PRESETS : LLM_PRESETS
          const label = role === "llm" ? "LLM" : role === "embed" ? "Embedding" : "Judge"

          return (
            <Card key={role}>
              <CardHeader>
                <CardTitle className="text-base">{label}</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="space-y-1.5">
                  <Label>Provider *</Label>
                  <Select
                    value={provider}
                    onValueChange={(p) => {
                      setProvider(p)
                      setModel(presets[p] ?? "")
                    }}
                  >
                    <SelectTrigger>
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
                  <Label>Model *</Label>
                  <Input value={model} onChange={(e) => setModel(e.target.value)} required />
                </div>
              </CardContent>
            </Card>
          )
        })}

        <Card>
          <CardHeader>
            <CardTitle className="text-base">Performance</CardTitle>
          </CardHeader>
          <CardContent className="grid grid-cols-2 gap-4">
            <div className="space-y-1.5">
              <Label htmlFor="batch_size">Batch Size</Label>
              <Input id="batch_size" type="number" value={batchSize} onChange={(e) => setBatchSize(Number(e.target.value))} min={1} />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="workers">Workers</Label>
              <Input id="workers" type="number" value={workers} onChange={(e) => setWorkers(Number(e.target.value))} min={1} />
            </div>
          </CardContent>
        </Card>

        <div className="flex gap-2">
          <Button type="submit" disabled={mutation.isPending}>
            {mutation.isPending ? "Creating..." : "Create"}
          </Button>
          <Button type="button" variant="outline" asChild>
            <Link to="/evaluations">Cancel</Link>
          </Button>
        </div>
      </form>
    </div>
  )
}
