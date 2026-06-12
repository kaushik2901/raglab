import { useState } from "react"
import { Link, useNavigate } from "react-router-dom"
import { useCreatePreprocess } from "@/hooks/useWorkflows"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { ChipInput } from "@/components/ChipInput"
import { Separator } from "@/components/ui/separator"
import { RiArrowLeftLine, RiGitBranchLine } from "@remixicon/react"

export default function ArtifactCreate() {
  const navigate = useNavigate()
  const mutation = useCreatePreprocess()

  const [repoUrl, setRepoUrl] = useState("https://gitlab.com/gitlab-com/content-sites/handbook.git")
  const [tag, setTag] = useState("")
  const [baseUrl, setBaseUrl] = useState("")
  const [includeDirs, setIncludeDirs] = useState<string[]>([])

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    mutation.mutate(
      {
        repo_url: repoUrl,
        tag,
        ...(baseUrl && { base_url: baseUrl }),
        ...(includeDirs.length > 0 && { include_dirs: includeDirs }),
      },
      { onSuccess: () => navigate("/artifacts") }
    )
  }

  return (
    <div className="max-w-2xl space-y-8">
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="icon" asChild className="size-8">
          <Link to="/artifacts">
            <RiArrowLeftLine className="size-4" />
          </Link>
        </Button>
        <div>
          <h1 className="text-2xl font-bold tracking-tight">New Artifact</h1>
          <p className="text-sm text-muted-foreground mt-1">
            Clone and preprocess a Git repository for indexing.
          </p>
        </div>
      </div>

      <Separator />

      <form onSubmit={handleSubmit} className="space-y-6">
        <Card>
          <CardHeader>
            <div className="flex items-center gap-2 mb-1">
              <div className="flex size-5 items-center justify-center rounded-full bg-primary/10">
                <span className="text-[10px] font-bold text-primary">1</span>
              </div>
              <CardTitle className="text-base">Repository</CardTitle>
            </div>
            <CardDescription>Source repository to clone and preprocess.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="repo_url">Repository URL</Label>
              <Input id="repo_url" value={repoUrl} onChange={(e) => setRepoUrl(e.target.value)} required />
            </div>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="tag">Tag</Label>
                <Input id="tag" value={tag} onChange={(e) => setTag(e.target.value)} placeholder="e.g. v2-june" required />
              </div>
              <div className="space-y-2">
                <Label htmlFor="base_url">Base URL <span className="text-muted-foreground font-normal">(optional)</span></Label>
                <Input id="base_url" value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} placeholder="https://handbook.gitlab.com" />
              </div>
            </div>
            <div className="space-y-2">
              <Label>Include directories <span className="text-muted-foreground font-normal">(optional)</span></Label>
              <ChipInput value={includeDirs} onChange={setIncludeDirs} placeholder="Type a path and press Enter..." />
              <p className="text-[11px] text-muted-foreground">Limit preprocessing to specific subdirectories. Leave empty to process all.</p>
            </div>
          </CardContent>
        </Card>

        <div className="flex gap-3 pt-2">
          <Button type="submit" disabled={mutation.isPending} size="lg">
            <RiGitBranchLine className="size-4" />
            {mutation.isPending ? "Cloning & Preprocessing..." : "Preprocess Repository"}
          </Button>
          <Button type="button" variant="outline" size="lg" asChild>
            <Link to="/artifacts">Cancel</Link>
          </Button>
        </div>
      </form>
    </div>
  )
}
