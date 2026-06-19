export interface ChatSourceDoc {
  document_path: string
  score: number
  source_url: string
}

export interface ChatTokenUsage {
  prompt: number
  completion: number
  total: number
}

export interface ChatResponse {
  answer: string
  conversation_id: string
  source_documents: ChatSourceDoc[]
  token_usage: ChatTokenUsage
  latency_ms: number
}

export interface ConversationMessage {
  id: string
  conversation_id: string
  role: "user" | "assistant"
  content: string
  sources: ChatSourceDoc[] | null
  token_usage: ChatTokenUsage | null
  created_at: string
}

export interface Conversation {
  id: string
  created_at: string
  messages: ConversationMessage[]
}
