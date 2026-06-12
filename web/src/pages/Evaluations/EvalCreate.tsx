import { useState } from "react"
import { Link, useNavigate } from "react-router-dom"
import { useIndexes } from "@/hooks/useIndexes"
import { useDatasets } from "@/hooks/useDatasets"
import { useCreateEval } from "@/hooks/useWorkflows"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { RiArrowLeftLine, RiLineChartLine } from "@remixicon/react"

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

function StepBadge({ num }: { num: number }) {
  return (
    <div className="flex size-5 items-center justify-center rounded-full bg-primary/10">
      <span className="text-[10px] font-bold text-primary">{num}</span>
    </div>
  )
}

export default function EvalCreate() {
  const navigate = useNavigate()
  const { data: indexes } = useIndexes()
  const { data: datasets } = useDatasets()
  const mutation = useCreateEval()

  const [indexTag, setIndexTag] = useState("")
  const [datasetPath, setDatasetPath] = useState("")
  const [tag, setTag] = useState("")
  const [queryStrategy, setQueryStrategy] = useState("naive-search")
  const [ksInput, setKsInput] = useState("1, 3, 5")
  const [llmProvider, setLlmProvider] = useState("openai")
  const [llmModel, setLlmModel] = useState("gpt-4o-mini")
  const [embedProvider, setEmbedProvider] = useState("openai")
  const [embedModel, setEmbedModel] = useState("text-embedding-3-small")
  const [judgeProvider, setJudgeProvider] = useState("openai")
  const [judgeModel, setJudgeModel] = useState("gpt-4o-mini")
  const [batchSize, setBatchSize] = useState(20)
  const [workers, setWorkers] = useState(5)

  const readyIndexes = (indexes ?? []).filter((i) => !("pending" in i && i.pending))

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const ks = ksInput.split(",").map((s) => parseInt(s.trim(), 10)).filter((n) => !isNaN(n) && n > 0)
    if (ks.length === 0) return
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
    <div className="max-w-2xl space-y-8">
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="icon" asChild className="size-8">
          <Link to="/evaluations">
            <RiArrowLeftLine className="size-4" />
          </Link>
        </Button>
        <div>
          <h1 className="text-2xl font-bold tracking-tight">New Evaluation</h1>
          <p className="text-sm text-muted-foreground mt-1">
            Run a RAG evaluation pipeline against an index using a question dataset.
          </p>
        </div>
      </div>

      <Separator />

      <form onSubmit={handleSubmit} className="space-y-6">
        <Card>
          <CardHeader>
            <div className="flex items-center gap-2 mb-1">
              <StepBadge num={1} />
              <CardTitle className="text-base">Data Sources</CardTitle>
            </div>
            <CardDescription>Select the vector index and question dataset for evaluation.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="index_tag">Index</Label>
                <Select value={indexTag} onValueChange={setIndexTag}>
                  <SelectTrigger id="index_tag">
                    <SelectValue placeholder="Select an index..." />
                  </SelectTrigger>
                  <SelectContent>
                    {readyIndexes.map((idx) => (
                      <SelectItem key={idx.name} value={idx.name}>{idx.name}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="dataset">Dataset</Label>
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
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <div className="flex items-center gap-2 mb-1">
              <StepBadge num={2} />
              <CardTitle className="text-base">Strategy & Identity</CardTitle>
            </div>
            <CardDescription>Configure the evaluation run identity and retrieval strategy.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="tag">Run Tag</Label>
              <Input id="tag" value={tag} onChange={(e) => setTag(e.target.value)} placeholder="e.g. eval-v1" required />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label>Query Strategy</Label>
                <Select value={queryStrategy} onValueChange={setQueryStrategy}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="naive-search">Naive Search (vector similarity)</SelectItem>
                    <SelectItem value="mmr-rerank">MMR Re-rank (diversity-aware)</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="ks">K Values</Label>
                <Input id="ks" value={ksInput} onChange={(e) => setKsInput(e.target.value)} placeholder="1, 3, 5, 10" />
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <div className="flex items-center gap-2 mb-1">
              <StepBadge num={3} />
              <CardTitle className="text-base">Model Configuration</CardTitle>
            </div>
            <CardDescription>Configure LLM, embedding, and judge provider/models.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {([
              { role: "llm", label: "LLM (Generator)", provider: llmProvider, model: llmModel, setProvider: setLlmProvider, setModel: setLlmModel, presets: LLM_PRESETS },
              { role: "embed", label: "Embedding Model", provider: embedProvider, model: embedModel, setProvider: setEmbedProvider, setModel: setEmbedModel, presets: EMBED_PRESETS },
              { role: "judge", label: "Judge Model", provider: judgeProvider, model: judgeModel, setProvider: setJudgeProvider, setModel: setJudgeModel, presets: LLM_PRESETS },
            ] as const).map((cfg) => (
              <div key={cfg.role} className="grid grid-cols-2 gap-4 p-3 rounded-lg bg-muted/30">
                <div className="space-y-2">
                  <Label>{cfg.label} Provider</Label>
                  <Select value={cfg.provider} onValueChange={(p) => { cfg.setProvider(p); cfg.setModel(cfg.presets[p] ?? "") }}>
                    <SelectTrigger>
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
                  <Label>{cfg.label} Model</Label>
                  <Input value={cfg.model} onChange={(e) => cfg.setModel(e.target.value)} required />
                </div>
              </div>
            ))}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <div className="flex items-center gap-2 mb-1">
              <StepBadge num={4} />
              <CardTitle className="text-base">Performance</CardTitle>
            </div>
            <CardDescription>Tune concurrency and batching for throughput.</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="batch_size">Batch Size</Label>
                <Input id="batch_size" type="number" value={batchSize} onChange={(e) => setBatchSize(Number(e.target.value))} min={1} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="workers">Workers</Label>
                <Input id="workers" type="number" value={workers} onChange={(e) => setWorkers(Number(e.target.value))} min={1} />
              </div>
            </div>
          </CardContent>
        </Card>

        <div className="flex gap-3 pt-2">
          <Button type="submit" disabled={mutation.isPending} size="lg">
            <RiLineChartLine className="size-4" />
            {mutation.isPending ? "Running Evaluation..." : "Run Evaluation"}
          </Button>
          <Button type="button" variant="outline" size="lg" asChild>
            <Link to="/evaluations">Cancel</Link>
          </Button>
        </div>
      </form>
    </div>
  )
}
