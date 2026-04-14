import { DiffBadge } from './DiffBadge'
import { useExplorerStore } from '../store/explorerStore'

export function CommitSlider() {
  const commits = useExplorerStore((s) => s.commits)
  const currentCommitIndex = useExplorerStore((s) => s.currentCommitIndex)
  const setCommitIndex = useExplorerStore((s) => s.setCommitIndex)
  const changedAtCommit = useExplorerStore((s) => s.changedAtCommit)

  if (commits.length === 0) {
    return (
      <footer className="h-[80px] border-t border-slate-800 bg-slate-950 px-4 py-3 text-sm text-slate-400">
        No commits loaded.
      </footer>
    )
  }

  const commit = commits[currentCommitIndex]
  const shortHash = commit.hash.slice(0, 7)
  const timeText = new Date(commit.timestamp).toLocaleString()

  return (
    <footer className="h-[80px] border-t border-slate-800 bg-slate-950 px-4 py-3">
      <div className="mb-2 flex items-center gap-3">
        <input
          type="range"
          min={0}
          max={Math.max(commits.length - 1, 0)}
          step={1}
          value={currentCommitIndex}
          className="h-2 w-full cursor-pointer accent-cyan-400"
          onChange={(event) => {
            const nextIndex = Number(event.target.value)
            void setCommitIndex(nextIndex)
          }}
        />
        <DiffBadge changed={changedAtCommit} />
      </div>
      <div className="truncate text-xs text-slate-300">
        {shortHash} - commit - {commit.actor} - {timeText}
      </div>
    </footer>
  )
}
