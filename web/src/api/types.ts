export interface ArtifactEntry {
  type: string
  tag: string
  file_count?: number | null
  created_at?: string
}

export interface CollectionInfo {
  name: string
  vector_count: number
  vector_size: number
  distance: string
}

export interface DatasetEntry {
  name: string
  size: number
  question_count: number
  created_at?: string
}

export interface JobEntry {
  id: number
  kind: string
  state: JobState
  tag: string
  attempt: number
  max_attempts: number
  created_at: string
  finalized_at?: string
  errors?: string[]
}

export type JobState =
  | "available"
  | "running"
  | "retrying"
  | "completed"
  | "cancelled"
  | "failed"

export const TERMINAL_STATES: JobState[] = ["completed", "cancelled", "failed"]

export interface RunSummary {
  id: string
  tag: string
  strategy: Record<string, unknown>
  metrics: Record<string, unknown> | null
  question_count: number
  created_at: string
}

export interface RunDetail extends RunSummary {
  questions: Record<string, unknown>[]
  total: number
}

export interface WorkflowResponse {
  job_id: number
  tag: string
  state: string
  created_at: string
}
