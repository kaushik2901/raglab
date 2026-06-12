import { Routes, Route } from "react-router-dom"
import { Layout } from "@/components/Layout"
import { CommandPalette } from "@/components/CommandPalette"
import Dashboard from "@/pages/Dashboard"
import Artifacts from "@/pages/Artifacts"
import ArtifactCreate from "@/pages/ArtifactCreate"
import Datasets from "@/pages/Datasets"
import Indexes from "@/pages/Indexes"
import IndexCreate from "@/pages/IndexCreate"
import Evaluations from "@/pages/Evaluations"
import RunList from "@/pages/Evaluations/RunList"
import RunDetail from "@/pages/Evaluations/RunDetail"
import RunCompare from "@/pages/Evaluations/RunCompare"
import EvalCreate from "@/pages/Evaluations/EvalCreate"
import Chat from "@/pages/Chat"

export function App() {
  return (
    <>
      <CommandPalette />
      <Routes>
        <Route element={<Layout />}>
          <Route index element={<Dashboard />} />
          <Route path="artifacts" element={<Artifacts />} />
          <Route path="artifacts/new" element={<ArtifactCreate />} />
          <Route path="datasets" element={<Datasets />} />
          <Route path="indexes" element={<Indexes />} />
          <Route path="indexes/new" element={<IndexCreate />} />
          <Route path="evaluations" element={<Evaluations />}>
            <Route index element={<RunList />} />
            <Route path="new" element={<EvalCreate />} />
            <Route path=":id" element={<RunDetail />} />
            <Route path=":id/compare" element={<RunCompare />} />
          </Route>
          <Route path="chat" element={<Chat />} />
        </Route>
      </Routes>
    </>
  )
}

export default App
