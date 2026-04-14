interface DiffBadgeProps {
  changed: string[]
}

export function DiffBadge({ changed }: DiffBadgeProps) {
  const title = changed.length > 0 ? changed.join('\n') : 'No changes at this commit'

  return (
    <div
      className="rounded-full border border-amber-400/50 bg-amber-500/15 px-2 py-1 text-xs text-amber-200"
      title={title}
    >
      {changed.length} changes
    </div>
  )
}
