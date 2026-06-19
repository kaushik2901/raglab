import { useState, useRef, useEffect, useCallback } from "react"
import { useChat } from "@ai-sdk/react"
import { DefaultChatTransport } from "ai"
import type { UIMessage } from "ai"
import { useIndexes } from "@/hooks/useIndexes"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { ScrollArea } from "@/components/ui/scroll-area"
import { RiSendPlaneFill, RiRobot2Line, RiUserLine, RiFileTextLine, RiChat3Line, RiStopFill } from "@remixicon/react"
import { cn } from "@/lib/utils"

const PROVIDERS = ["openai", "gemini", "openrouter", "lmstudio"] as const
const EMBED_PROVIDERS = ["openai", "gemini", "openrouter", "lmstudio"] as const

const MODEL_DEFAULTS: Record<string, string> = {
  openai: "gpt-4o-mini",
  gemini: "gemini-2.0-flash",
  openrouter: "openai/gpt-4o-mini",
  lmstudio: "local-model",
}

const EMBED_MODEL_DEFAULTS: Record<string, string> = {
  openai: "text-embedding-3-small",
  gemini: "text-embedding-004",
  openrouter: "openai/text-embedding-3-small",
  lmstudio: "text-embedding-nomic-embed-text-v1.5",
}

function getTextContent(msg: UIMessage): string {
  return msg.parts
    .filter((p): p is { type: "text"; text: string } => p.type === "text")
    .map((p) => p.text)
    .join("")
}

function getSourceDocs(msg: UIMessage) {
  return msg.parts.filter(
    (p): p is { type: "source-document"; sourceId: string; mediaType: string; title: string; filename?: string } =>
      p.type === "source-document",
  )
}

export default function Chat() {
  const { data: indexes } = useIndexes()

  const [tag, setTag] = useState("")
  const [provider, setProvider] = useState<string>("openai")
  const [model, setModel] = useState(MODEL_DEFAULTS.openai)
  const [embedProvider, setEmbedProvider] = useState<string>("openai")
  const [embedModel, setEmbedModel] = useState(EMBED_MODEL_DEFAULTS.openai)
  const [topK, setTopK] = useState(5)
  const [temperature, setTemperature] = useState(0.3)
  const [maxTokens, setMaxTokens] = useState(1024)
  const [input, setInput] = useState("")

  const settingsRef = useRef({ tag: "", topK: 5, temperature: 0.3, maxTokens: 1024, provider: "openai", model: MODEL_DEFAULTS.openai, embedProvider: "openai", embedModel: EMBED_MODEL_DEFAULTS.openai })
  settingsRef.current = { tag, topK, temperature, maxTokens, provider, model, embedProvider, embedModel }

  const scrollRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  const customFetch = useCallback(
    async (url: string | URL | Request, options?: RequestInit): Promise<Response> => {
      const bodyStr = options?.body?.toString() ?? "{}"
      let body: Record<string, unknown>
      try { body = JSON.parse(bodyStr) } catch { body = {} }

      const s = settingsRef.current
      const transformedBody = {
        messages: body.messages,
        tag: s.tag,
        top_k: s.topK,
        temperature: s.temperature,
        max_tokens: s.maxTokens,
        llm_provider: s.provider,
        llm_model: s.model,
        embedding_provider: s.embedProvider,
        embedding_model: s.embedModel,
      }

      return fetch(url, {
        ...options,
        headers: {
          ...options?.headers,
          "Content-Type": "application/json",
        },
        body: JSON.stringify(transformedBody),
      })
    },
    [],
  )

  const { messages, sendMessage, status, stop, error } = useChat({
    transport: new DefaultChatTransport({
      api: "/api/v1/chat/stream",
      fetch: customFetch,
    }),
    onError: (err: Error) => {
      console.error("Chat error:", err)
    },
  })

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [messages])

  const isLoading = status === "submitted" || status === "streaming"

  const handleFormSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!input.trim() || !tag || isLoading) return
    sendMessage({ text: input.trim() })
    setInput("")
  }

  const handleProviderChange = (value: string) => {
    setProvider(value)
    setModel(MODEL_DEFAULTS[value] ?? "")
  }

  const handleEmbedProviderChange = (value: string) => {
    setEmbedProvider(value)
    setEmbedModel(EMBED_MODEL_DEFAULTS[value] ?? "")
  }

  const readyIndexes = (indexes ?? []).filter((i) => !("pending" in i && i.pending))

  return (
    <div className="space-y-4 h-[calc(100vh-6rem)] flex flex-col">
      <div className="shrink-0">
        <h1 className="text-2xl font-bold tracking-tight">Chat</h1>
        <p className="text-sm text-muted-foreground mt-1">
          RAG-powered Q&A over your indexed handbook documents.
        </p>
      </div>

      <Card className="shrink-0">
        <CardContent className="pt-4">
          <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-3">
            <div className="space-y-1">
              <Label className="text-[11px]">Index</Label>
              <Select value={tag} onValueChange={setTag}>
                <SelectTrigger className="h-8 text-xs">
                  <SelectValue placeholder="Select index..." />
                </SelectTrigger>
                <SelectContent>
                  {readyIndexes.map((idx) => (
                    <SelectItem key={idx.name} value={idx.name}>{idx.name}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <Label className="text-[11px]">LLM Provider</Label>
              <Select value={provider} onValueChange={handleProviderChange}>
                <SelectTrigger className="h-8 text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {PROVIDERS.map((p) => (
                    <SelectItem key={p} value={p}>{p}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <Label className="text-[11px]">LLM Model</Label>
              <Input value={model} onChange={(e) => setModel(e.target.value)} className="h-8 text-xs" placeholder="gpt-4o-mini" />
            </div>
            <div className="space-y-1">
              <Label className="text-[11px]">Embed Provider</Label>
              <Select value={embedProvider} onValueChange={handleEmbedProviderChange}>
                <SelectTrigger className="h-8 text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {EMBED_PROVIDERS.map((p) => (
                    <SelectItem key={p} value={p}>{p}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <Label className="text-[11px]">Embed Model</Label>
              <Input value={embedModel} onChange={(e) => setEmbedModel(e.target.value)} className="h-8 text-xs" placeholder="text-embedding-3-small" />
            </div>
            <div className="space-y-1">
              <Label className="text-[11px]">Top-K</Label>
              <Input type="number" value={topK} onChange={(e) => setTopK(Number(e.target.value))} min={1} max={20} className="h-8 text-xs" />
            </div>
            <div className="space-y-1">
              <Label className="text-[11px]">Temperature</Label>
              <Input type="number" value={temperature} onChange={(e) => setTemperature(Number(e.target.value))} min={0.1} max={2} step={0.1} className="h-8 text-xs" />
            </div>
            <div className="space-y-1">
              <Label className="text-[11px]">Max Tokens</Label>
              <Input type="number" value={maxTokens} onChange={(e) => setMaxTokens(Number(e.target.value))} min={1} max={32768} className="h-8 text-xs" />
            </div>
          </div>
        </CardContent>
      </Card>

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
              {messages.map((msg, i) => {
                const isAssistant = msg.role === "assistant"
                const text = getTextContent(msg)
                const sources = isAssistant ? getSourceDocs(msg) : []
                const isLastAssistant = isAssistant && i === messages.length - 1 && isLoading

                return (
                  <div key={msg.id || i} className={cn("flex gap-3", msg.role === "user" ? "justify-end" : "justify-start")}>
                    <div className={cn(
                      "flex size-7 items-center justify-center rounded-full shrink-0 mt-0.5",
                      msg.role === "user" ? "bg-primary text-primary-foreground order-2" : "bg-muted",
                    )}>
                      {msg.role === "user" ? <RiUserLine className="size-3.5" /> : <RiRobot2Line className="size-3.5" />}
                    </div>
                    <div className={cn(
                      "rounded-lg px-4 py-3 max-w-[80%]",
                      msg.role === "user" ? "bg-primary text-primary-foreground" : "bg-card border",
                    )}>
                      <div className="text-sm leading-relaxed whitespace-pre-wrap">
                        {text || (isLastAssistant ? (
                          <span className="inline-flex gap-1">
                            <span className="animate-pulse">{"\u25CF"}</span>
                            <span className="animate-pulse" style={{ animationDelay: "0.2s" }}>{"\u25CF"}</span>
                            <span className="animate-pulse" style={{ animationDelay: "0.4s" }}>{"\u25CF"}</span>
                          </span>
                        ) : null)}
                      </div>

                      {isAssistant && sources.length > 0 && (
                        <div className="mt-3 pt-3 border-t">
                          <p className="text-[10px] font-semibold text-muted-foreground uppercase tracking-wider mb-2">
                            Sources
                          </p>
                          <div className="space-y-1.5">
                            {sources.map((src, si) => (
                              <div key={si} className="flex items-start gap-2 text-xs">
                                <RiFileTextLine className="size-3 mt-0.5 text-muted-foreground shrink-0" />
                                <div className="min-w-0">
                                  <span className="font-medium">{src.filename ?? src.title}</span>
                                </div>
                              </div>
                            ))}
                          </div>
                        </div>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>
          </ScrollArea>
        )}

        {error && (
          <div className="shrink-0 px-4 py-2 bg-destructive/10 border-t border-destructive/20">
            <p className="text-xs text-destructive">{error.message || "An error occurred"}</p>
          </div>
        )}

        <form onSubmit={handleFormSubmit} className="shrink-0 border-t p-4 bg-background">
          <div className="flex gap-2 max-w-3xl mx-auto">
            <Input
              ref={inputRef}
              value={input}
              onChange={(e) => setInput(e.target.value)}
              placeholder={tag ? "Ask a question..." : "Select an index first..."}
              disabled={!tag || isLoading}
              className="flex-1"
            />
            {isLoading ? (
              <Button type="button" variant="outline" onClick={() => stop?.()}>
                <RiStopFill className="size-4" />
              </Button>
            ) : (
              <Button type="submit" disabled={!tag || isLoading || !input.trim()}>
                <RiSendPlaneFill className="size-4" />
              </Button>
            )}
          </div>
        </form>
      </div>
    </div>
  )
}
