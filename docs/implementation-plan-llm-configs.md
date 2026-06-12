# LLM & Model Management Module — Implementation Plan

## Goal

Create a central management system for LLM provider/model pairs, replacing hardcoded presets and free-text model inputs with a DB-backed CRUD interface and async dropdown selects throughout the UI.

---

## Phase 1: Database & Backend API

### 1.1 Migration (`internal/db/migrations/002_llm_configs.sql`)

```sql
CREATE TABLE IF NOT EXISTS llm_configs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider    TEXT NOT NULL,
    model       TEXT NOT NULL,
    label       TEXT NOT NULL,
    config_type TEXT NOT NULL DEFAULT 'llm',
    is_default  BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_configs_provider_model ON llm_configs(provider, model);
CREATE INDEX IF NOT EXISTS idx_llm_configs_type ON llm_configs(config_type);
```

- `provider`: one of `openai`, `gemini`, `openrouter`, `lmstudio`
- `model`: model identifier (e.g. `gpt-4o-mini`, `text-embedding-3-small`)
- `label`: human-readable label (e.g. `OpenAI GPT-4o Mini`)
- `config_type`: discriminator — `llm`, `embedding`, `judge`
- `is_default`: mark default per `(provider, config_type)` group; application enforces at most one
- API keys stay in env vars (existing `config.ResolveProviderConfig` pattern)

### 1.2 Go Types (`internal/api/types.go`)

Add:

```go
type LLMConfig struct {
    ID        string `json:"id"`
    Provider  string `json:"provider"`
    Model     string `json:"model"`
    Label     string `json:"label"`
    ConfigType string `json:"config_type"`
    IsDefault bool   `json:"is_default"`
    CreatedAt string `json:"created_at"`
}

type CreateLLMConfigRequest struct {
    Provider    string `json:"provider"`
    Model       string `json:"model"`
    Label       string `json:"label"`
    ConfigType  string `json:"config_type"`
    IsDefault   bool   `json:"is_default"`
}

type UpdateLLMConfigRequest struct {
    Provider   *string `json:"provider,omitempty"`
    Model      *string `json:"model,omitempty"`
    Label      *string `json:"label,omitempty"`
    ConfigType *string `json:"config_type,omitempty"`
    IsDefault  *bool   `json:"is_default,omitempty"`
}
```

### 1.3 Service / Store (`internal/api/service_llm_config.go`)

Interface:

```go
type LLMConfigStore interface {
    List(ctx context.Context, configType string) ([]LLMConfig, error)
    Get(ctx context.Context, id string) (*LLMConfig, error)
    Create(ctx context.Context, req CreateLLMConfigRequest) (*LLMConfig, error)
    Update(ctx context.Context, id string, req UpdateLLMConfigRequest) (*LLMConfig, error)
    Delete(ctx context.Context, id string) error
}
```

`PgLLMConfigStore` with `pgxpool.Pool`:

- `List`: `SELECT * FROM llm_configs WHERE ($1 = '' OR config_type = $1) ORDER BY provider, label`
- `Create`: insert with `RETURNING *`; if `is_default`, unset others in the same `(provider, config_type)` group within a transaction
- `Update`: partial update building SET clause dynamically; handle `is_default` toggle same as create
- `Delete`: `DELETE FROM llm_configs WHERE id = $1`

### 1.4 Router (`internal/api/router_llm_config.go`)

| Method | Path | Handler |
|---|---|---|
| `GET` | `/api/v1/models` | `listHandler` — optional `?type=llm\|embedding\|judge` filter |
| `GET` | `/api/v1/models/{id}` | `getHandler` |
| `POST` | `/api/v1/models` | `createHandler` |
| `PUT` | `/api/v1/models/{id}` | `updateHandler` |
| `DELETE` | `/api/v1/models/{id}` | `deleteHandler` |

Register in `server.go` during `NewServer`.

---

## Phase 2: Frontend — API Client & Hooks

### 2.1 Types (`web/src/api/types.ts`)

```typescript
export interface LLMConfig {
  id: string
  provider: string
  model: string
  label: string
  config_type: "llm" | "embedding" | "judge"
  is_default: boolean
  created_at: string
}
```

### 2.2 API functions (`web/src/api/models.ts`)

```typescript
export async function fetchModels(type?: string): Promise<LLMConfig[]>
export async function fetchModel(id: string): Promise<LLMConfig>
export async function createModel(data: CreateLLMConfigPayload): Promise<LLMConfig>
export async function updateModel(id: string, data: UpdateLLMConfigPayload): Promise<LLMConfig>
export async function deleteModel(id: string): Promise<void>
```

### 2.3 TanStack Query hooks (`web/src/hooks/useModels.ts`)

- `useModels(type?)` — `useQuery` keyed on `["models", type]`
- `useModel(id)` — single fetch
- `useCreateModel()` — invalidates `["models"]`
- `useUpdateModel()` — invalidates `["models"]`
- `useDeleteModel()` — invalidates `["models"]`

---

## Phase 3: Frontend — Management Pages

### 3.1 Route registration (`web/src/App.tsx`)

```
/settings/models          → Models (list)
/settings/models/new      → ModelCreate (form)
```

Add a "Models" link in the sidebar navigation (Layout.tsx) under a "Settings" section.

### 3.2 List page (`web/src/pages/Settings/Models.tsx`)

- Tabs or segmented control to filter by `config_type` (All / LLM / Embedding / Judge)
- Table with columns: Provider, Model, Label, Type, Default badge, Actions (Edit/Delete)
- "Add Model" button → navigates to `/settings/models/new`
- Delete with confirmation dialog (`ConfirmDialog.tsx`)
- Default toggle: inline action that calls update API

### 3.3 Create form (`web/src/pages/Settings/ModelCreate.tsx`)

- Provider: dropdown (`openai`, `gemini`, `openrouter`, `lmstudio`)
- Model: text input
- Label: text input
- Config Type: dropdown (`llm`, `embedding`, `judge`)
- Set as Default: checkbox
- Save button → creates via API → navigates back to list

### 3.4 Inline edit (or edit page)

Same form as create, pre-filled with existing values. Could be a dialog or separate page.

---

## Phase 4: Frontend — Refactor Existing Forms

### 4.1 EvalCreate.tsx

Replace the three model sections (LLM, Embedding, Judge) with dropdowns populated from `useModels(type)`:

```tsx
const { data: llmModels } = useModels("llm")
const { data: embedModels } = useModels("embedding")
const { data: judgeModels } = useModels("judge")

// Each dropdown group:
<Select value={selectedId} onValueChange={setSelectedId}>
  {models?.map(m => (
    <SelectItem key={m.id} value={m.id}>
      {m.label} ({m.provider})
    </SelectItem>
  ))}
</Select>
```

On submit, look up the selected `LLMConfig` by ID and extract `provider` + `model` for the request body. This preserves backend compatibility.

### 4.2 Chat.tsx

Same pattern — replace LLM and Embedding free-text inputs with async model dropdowns.

### 4.3 IndexCreate.tsx (if applicable)

Replace embedding model text input with model dropdown.

---

## Phase 5: Edge Cases & Polish

- **Empty state**: Show helpful message when no models are configured ("Add your first model to get started")
- **Validation**: Prevent duplicate `(provider, model, config_type)` combos at API level
- **Default enforcement**: When deleting a default model, either promote another or allow none
- **LB integration**: EvalCreate should show `llm` and `judge` model configs for LLM/Judge fields; `embedding` model configs for Embedding field
- **Error handling**: Toast notifications on create/update/delete failures

---

## Files Created / Modified

| Action | Path |
|---|---|
| CREATE | `internal/db/migrations/002_llm_configs.sql` |
| CREATE | `internal/api/service_llm_config.go` |
| CREATE | `internal/api/router_llm_config.go` |
| MODIFY | `internal/api/types.go` (add LLMConfig types) |
| MODIFY | `internal/api/server.go` (register routes) |
| CREATE | `web/src/api/models.ts` |
| CREATE | `web/src/hooks/useModels.ts` |
| CREATE | `web/src/pages/Settings/Models.tsx` |
| CREATE | `web/src/pages/Settings/ModelCreate.tsx` |
| MODIFY | `web/src/api/types.ts` (add LLMConfig interface) |
| MODIFY | `web/src/App.tsx` (add routes) |
| MODIFY | `web/src/components/Layout.tsx` (add nav link) |
| MODIFY | `web/src/pages/Evaluations/EvalCreate.tsx` (model dropdowns) |
| MODIFY | `web/src/pages/Chat.tsx` (model dropdowns) |
