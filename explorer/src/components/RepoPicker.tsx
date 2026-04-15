import { useEffect } from 'react'
import { FolderGit2 } from 'lucide-react'
import { useExplorerStore } from '../store/explorerStore'

export function RepoPicker() {
  const repos = useExplorerStore((s) => s.repos)
  const selectedRepo = useExplorerStore((s) => s.selectedRepo)
  const loadRepos = useExplorerStore((s) => s.loadRepos)
  const selectRepo = useExplorerStore((s) => s.selectRepo)

  useEffect(() => {
    void loadRepos()
  }, [loadRepos])

  return (
    <aside className="w-[200px] border-r border-slate-800 bg-slate-950/70 p-3">
      <h2 className="mb-3 text-xs font-semibold uppercase tracking-wide text-slate-400">Repositories</h2>
      <div className="space-y-2 overflow-y-auto">
        {repos.map((repo) => (
          <button
            key={repo.id}
            className={`w-full rounded-lg border px-3 py-2 text-left text-sm transition ${
              selectedRepo === repo.id
                ? 'border-cyan-400 bg-cyan-500/10 text-cyan-200'
                : 'border-slate-800 bg-slate-900 text-slate-300 hover:border-slate-700 hover:bg-slate-800/80'
            }`}
            onClick={() => void selectRepo(repo.id)}
          >
            <div className="flex items-center gap-2">
              <FolderGit2 size={14} />
              <span className="truncate">{repo.name}</span>
            </div>
          </button>
        ))}
      </div>
    </aside>
  )
}
