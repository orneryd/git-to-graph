import { useMemo } from 'react'
import { GitBranchPlus, LoaderCircle } from 'lucide-react'
import { RepoPicker } from './components/RepoPicker'
import { CommitSlider } from './components/CommitSlider'
import { GraphCanvas } from './components/GraphCanvas'
import { DetailPanel } from './components/DetailPanel'
import { useExplorerStore } from './store/explorerStore'

function Header() {
  const repos = useExplorerStore((s) => s.repos)
  const selectedRepo = useExplorerStore((s) => s.selectedRepo)
  const selectRepo = useExplorerStore((s) => s.selectRepo)
  const selectedName = useMemo(() => repos.find((r) => r.id === selectedRepo)?.name ?? 'Select repo', [repos, selectedRepo])

  return (
    <header className="flex h-12 items-center justify-between border-b border-slate-800 bg-slate-900 px-4">
      <div className="flex items-center gap-2 text-sm font-semibold text-slate-100">
        <GitBranchPlus size={16} className="text-cyan-300" />
        Graph Explorer
      </div>

      <label className="flex items-center gap-2 text-xs text-slate-300">
        <span className="text-slate-400">Repo</span>
        <select
          value={selectedRepo ?? ''}
          className="rounded border border-slate-700 bg-slate-950 px-2 py-1 text-xs text-slate-100"
          onChange={(event) => {
            if (event.target.value) {
              void selectRepo(event.target.value)
            }
          }}
        >
          {selectedRepo === null ? <option value="">Select repo</option> : null}
          {repos.map((repo) => (
            <option key={repo.id} value={repo.id}>
              {repo.name}
            </option>
          ))}
        </select>
        <span className="text-slate-500">{selectedName}</span>
      </label>
    </header>
  )
}

function BackButton() {
  const viewMode = useExplorerStore((s) => s.viewMode)
  const backToProject = useExplorerStore((s) => s.backToProject)

  if (viewMode !== 'symbol') return null

  return (
    <button
      className="absolute left-3 top-3 z-10 rounded border border-slate-700 bg-slate-950/90 px-2 py-1 text-xs text-slate-200 hover:bg-slate-900"
      onClick={() => void backToProject()}
    >
      ← Back to project
    </button>
  )
}

function App() {
  const isLoading = useExplorerStore((s) => s.isLoading)

  return (
    <div className="flex h-full w-full flex-col bg-slate-950 text-slate-100">
      <Header />
      <div className="flex min-h-0 flex-1">
        <RepoPicker />
        <main className="relative min-h-0 min-w-0 flex-1 bg-slate-950">
          <BackButton />
          <GraphCanvas />
          {isLoading ? (
            <div className="pointer-events-none absolute inset-0 grid place-items-center bg-slate-950/35">
              <LoaderCircle className="h-7 w-7 animate-spin text-cyan-300" />
            </div>
          ) : null}
        </main>
        <DetailPanel />
      </div>
      <CommitSlider />
    </div>
  )
}

export default App
