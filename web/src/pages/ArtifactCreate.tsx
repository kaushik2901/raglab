import { useState } from "react"
import { Link, useNavigate } from "react-router-dom"
import { useCreatePreprocess } from "@/hooks/useWorkflows"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { ChipInput } from "@/components/ChipInput"
import { RiArrowLeftLine } from "@remixicon/react"

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
      {
        onSuccess: () => navigate("/artifacts"),
      }
    )
  }

  return (
    <div className="mx-auto max-w-xl space-y-6">
      <div className="flex items-center gap-2">
        <Button variant="ghost" size="icon" asChild className="size-8">
          <Link to="/artifacts">
            <RiArrowLeftLine className="size-4" />
          </Link>
        </Button>
        <h2 className="text-lg font-semibold">New Artifact</h2>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Repository details</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="repo_url">repo_url *</Label>
              <Input
                id="repo_url"
                value={repoUrl}
                onChange={(e) => setRepoUrl(e.target.value)}
                required
              />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="tag">tag *</Label>
              <Input
                id="tag"
                value={tag}
                onChange={(e) => setTag(e.target.value)}
                placeholder="e.g. v2-june"
                required
              />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="base_url">base_url (optional)</Label>
              <Input
                id="base_url"
                value={baseUrl}
                onChange={(e) => setBaseUrl(e.target.value)}
                placeholder="https://handbook.gitlab.com"
              />
            </div>

            <div className="space-y-1.5">
              <Label>include_dirs (optional)</Label>
              <ChipInput
                value={includeDirs}
                onChange={setIncludeDirs}
                placeholder="Type a directory path and press Enter..."
              />
            </div>

            <div className="flex gap-2 pt-2">
              <Button type="submit" disabled={mutation.isPending}>
                {mutation.isPending ? "Creating..." : "Create"}
              </Button>
              <Button type="button" variant="outline" asChild>
                <Link to="/artifacts">Cancel</Link>
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
