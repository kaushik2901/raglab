import { useState, useRef, useEffect } from "react"
import { useIndexes } from "@/hooks/useIndexes"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Badge } from "@/components/ui/badge"
import { ScrollArea } from "@/components/ui/scroll-area"
import { RiSendPlaneFill, RiRobot2Line, RiUserLine, RiFileTextLine, RiChat3Line, RiSettings3Line } from "@remixicon/react"
import { cn } from "@/lib/utils"

interface Message {
  role: "user" | "assistant"
  content: string
  sources?: SourceDoc[]
  tokens?: TokenUsage
  latencyMs?: number
}

interface SourceDoc {
  path: string
  score: number
  content_snippet: string
}

interface TokenUsage {
  prompt_tokens: number
  completion_tokens: number
}

const PROVIDERS = ["openai", "gemini", "openrouter", "lmstudio"] as const

export default function Chat() {
  const { data: indexes } = useIndexes()

  const [tag, setTag] = useState("")
  const [provider, setProvider] = useState("openai")
  const [model] = useState("gpt-4o-mini")
  const [embedProvider] = useState("openai")
  const [embedModel] = useState("text-embedding-3-small")
  const [topK, setTopK] = useState(5)
  const [temperature, setTemperature] = useState(0.3)
  const [maxTokens] = useState(1024)
  const [query, setQuery] = useState("")
  const [loading, setLoading] = useState(false)
  const [messages, setMessages] = useState<Message[]>([])
  const [showSettings, setShowSettings] = useState(false)

  const scrollRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [messages])

  const handleSubmit = async (e?: React.FormEvent) => {
    e?.preventDefault()
    if (!query.trim() || !tag || loading) return

    const userMsg: Message = { role: "user", content: query.trim() }
    setMessages((prev) => [...prev, userMsg])
    setQuery("")
    setLoading(true)

    const assistantMsg: Message = { role: "assistant", content: "" }
    setMessages((prev) => [...prev, assistantMsg])

    try {
      const resp = await fetch("/api/v1/chat/chat/stream", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          tag,
          query: userMsg.content,
          top_k: topK,
          temperature,
          max_tokens: maxTokens,
          llm_provider: provider,
          llm_model: model,
          embedding_provider: embedProvider,
          embedding_model: embedModel,
        }),
      })

      if (!resp.ok) throw new Error(`HTTP ${resp.status}`)

      const reader = resp.body?.getReader()
      if (!reader) throw new Error("No response body")

      const decoder = new TextDecoder()
      let buffer = ""

      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        buffer += decoder.decode(value, { stream: true })
        const lines = buffer.split("\n")
        buffer = lines.pop() ?? ""

        for (const line of lines) {
          const trimmed = line.trim()
          if (!trimmed) continue

          if (trimmed.startsWith("event: ")) {
          } else if (trimmed.startsWith("data: ")) {
            try {
              const data = JSON.parse(trimmed.slice(6))

              if (data.results) {
                setMessages((prev) => {
                  const next = [...prev]
                  const last = next[next.length - 1]
                  if (last && last.role === "assistant") {
                    next[next.length - 1] = { ...last, sources: data.results }
                  }
                  return next
                })
              } else if (data.token) {
                setMessages((prev) => {
                  const next = [...prev]
                  const last = next[next.length - 1]
                  if (last && last.role === "assistant") {
                    next[next.length - 1] = { ...last, content: last.content + data.token }
                  }
                  return next
                })
              } else if (data.source_documents || data.tokens) {
                setMessages((prev) => {
                  const next = [...prev]
                  const last = next[next.length - 1]
                  if (last && last.role === "assistant") {
                    next[next.length - 1] = {
                      ...last,
                      sources: data.source_documents ?? last.sources,
                      tokens: data.tokens ?? last.tokens,
                      latencyMs: data.latency_ms ?? last.latencyMs,
                    }
                  }
                  return next
                })
              }
            } catch {
              // skip parse errors
            }
          }
        }
      }
    } catch (err: unknown) {
      setMessages((prev) => {
        const next = [...prev]
        const last = next[next.length - 1]
        if (last && last.role === "assistant") {
          next[next.length - 1] = {
            ...last,
            content: `Error: ${err instanceof Error ? err.message : "Unknown error"}`,
          }
        }
        return next
      })
    } finally {
      setLoading(false)
    }
  }

  const readyIndexes = (indexes ?? []).filter((i) => !("pending" in i && i.pending))

  return (
    <div className="space-y-6 h-[calc(100vh-6rem)] flex flex-col">
      <div className="flex items-center justify-between shrink-0">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Chat</h1>
          <p className="text-sm text-muted-foreground mt-1">
            RAG-powered Q&A over your indexed handbook documents.
          </p>
        </div>
        <Button variant="outline" size="icon" onClick={() => setShowSettings(!showSettings)} className="size-9">
          <RiSettings3Line className="size-4" />
        </Button>
      </div>

      {showSettings && (
        <Card className="shrink-0">
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium">Configuration</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <div className="space-y-1.5">
                <Label className="text-xs">Index</Label>
                <Select value={tag} onValueChange={setTag}>
                  <SelectTrigger className="h-9 text-sm">
                    <SelectValue placeholder="Select index..." />
                  </SelectTrigger>
                  <SelectContent>
                    {readyIndexes.map((idx) => (
                      <SelectItem key={idx.name} value={idx.name}>{idx.name}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1.5">
                <Label className="text-xs">LLM Provider</Label>
                <Select value={provider} onValueChange={setProvider}>
                  <SelectTrigger className="h-9 text-sm">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {PROVIDERS.map((p) => (
                      <SelectItem key={p} value={p}>{p}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1.5">
                <Label className="text-xs">Top-K</Label>
                <Input type="number" value={topK} onChange={(e) => setTopK(Number(e.target.value))} min={1} max={20} className="h-9" />
              </div>
              <div className="space-y-1.5">
                <Label className="text-xs">Temperature</Label>
                <Input type="number" value={temperature} onChange={(e) => setTemperature(Number(e.target.value))} min={0.1} max={2} step={0.1} className="h-9" />
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      <div className="flex-1 min-h-0 flex flex-col border rounded-lg bg-muted/10 overflow-hidden">
        {messages.length === 0 ? (
          <div className="flex-1 flex flex-col items-center justify-center text-center p-8">
            <div className="flex size-12 items-center justify-center rounded-full bg-primary/10 mb-4">
              <RiChat3Line className="size-6 text-primary" />
            </div>
            <p className="text-sm font-medium">Start a conversation</p>
            <p className="text-xs text-muted-foreground mt-1 max-w-md">
              Select an index above, then ask a question about your handbook documents.
            </p>
          </div>
        ) : (
          <ScrollArea className="flex-1 p-4" ref={scrollRef}>
            <div className="space-y-6 max-w-3xl mx-auto">
              {messages.map((msg, i) => (
                <div key={i} className={cn("flex gap-3", msg.role === "user" ? "justify-end" : "justify-start")}>
                  <div className={cn(
                    "flex size-7 items-center justify-center rounded-full shrink-0 mt-0.5",
                    msg.role === "user" ? "bg-primary text-primary-foreground order-2" : "bg-muted"
                  )}>
                    {msg.role === "user" ? <RiUserLine className="size-3.5" /> : <RiRobot2Line className="size-3.5" />}
                  </div>
                  <div className={cn(
                    "rounded-lg px-4 py-3 max-w-[80%]",
                    msg.role === "user" ? "bg-primary text-primary-foreground" : "bg-card border"
                  )}>
                    <div className="text-sm leading-relaxed whitespace-pre-wrap">
                      {msg.content || (loading && msg.role === "assistant" ? (
                        <span className="inline-flex gap-1">
                          <span className="animate-pulse">{"\u25CF"}</span>
                          <span className="animate-pulse" style={{ animationDelay: "0.2s" }}>{"\u25CF"}</span>
                          <span className="animate-pulse" style={{ animationDelay: "0.4s" }}>{"\u25CF"}</span>
                        </span>
                      ) : null)}
                    </div>

                    {msg.sources && msg.sources.length > 0 && msg.role === "assistant" && (
                      <div className="mt-3 pt-3 border-t">
                        <p className="text-[10px] font-semibold text-muted-foreground uppercase tracking-wider mb-2">
                          Sources
                        </p>
                        <div className="space-y-1.5">
                          {msg.sources.map((src, si) => (
                            <div key={si} className="flex items-start gap-2 text-xs">
                              <RiFileTextLine className="size-3 mt-0.5 text-muted-foreground shrink-0" />
                              <div className="min-w-0">
                                <span className="font-medium">{src.path}</span>
                                {src.score != null && (
                                  <Badge variant="outline" className="ml-2 text-[10px] px-1 py-0 h-4">
                                    {src.score.toFixed(3)}
                                  </Badge>
                                )}
                                {src.content_snippet && (
                                  <p className="text-muted-foreground mt-0.5 line-clamp-2">{src.content_snippet}</p>
                                )}
                              </div>
                            </div>
                          ))}
                        </div>
                        {msg.tokens && (
                          <div className="flex gap-4 mt-2 text-[10px] text-muted-foreground">
                            <span>Prompt: {msg.tokens.prompt_tokens}</span>
                            <span>Completion: {msg.tokens.completion_tokens}</span>
                            {msg.latencyMs != null && <span>{(msg.latencyMs / 1000).toFixed(1)}s</span>}
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </ScrollArea>
        )}

        <form onSubmit={handleSubmit} className="shrink-0 border-t p-4 bg-background">
          <div className="flex gap-2 max-w-3xl mx-auto">
            <Input
              ref={inputRef}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={tag ? "Ask a question..." : "Select an index first..."}
              disabled={!tag || loading}
              className="flex-1"
            />
            <Button type="submit" disabled={!tag || loading || !query.trim()}>
              <RiSendPlaneFill className="size-4" />
            </Button>
          </div>
        </form>
      </div>
    </div>
  )
}
