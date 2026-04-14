import { useEffect, useMemo, useState } from 'react'
import { executeCypher } from '../api/client'
import { symbolsInFileAtTimestampQuery } from '../api/queries'
import { useExplorerStore } from '../store/explorerStore'
import { asString, parseValue } from '../utils/parseValue'

interface SymbolSummary {
  id: string
  name: string
  kind: string
}

export function DetailPanel() {
  const nodes = useExplorerStore((s) => s.nodes)
  const commits = useExplorerStore((s) => s.commits)
  const currentCommitIndex = useExplorerStore((s) => s.currentCommitIndex)
  const database = useExplorerStore((s) => s.database)
  const selectedNodeId = useExplorerStore((s) => s.selectedNodeId)
  const selectedNode = useMemo(() => nodes.find((n) => n.id === selectedNodeId) ?? null, [nodes, selectedNodeId])
  const [symbols, setSymbols] = useState<SymbolSummary[]>([])

  useEffect(() => {
    const loadSymbols = async (): Promise<void> => {
      setSymbols([])
      if (!selectedNode || selectedNode.kind !== 'file') return

      const path = asString(selectedNode.properties.path)
      const commit = commits[currentCommitIndex]
      if (!path || !commit) return

      try {
        const result = await executeCypher(
          symbolsInFileAtTimestampQuery(),
          { timestamp: commit.timestamp, filePath: path },
          database,
        )

        const next = (result.results[0]?.data ?? []).map((row) => {
          const parsed = parseValue(row.row[2])
          return {
            id: asString(row.row[0]) ?? asString(parsed.id) ?? '',
            name: asString(parsed.name) ?? 'unknown',
            kind: asString(row.row[3]) ?? asString(parsed.kind) ?? 'symbol',
          }
        }).filter((item) => item.id)

        setSymbols(next)
      } catch {
        setSymbols([])
      }
    }

    void loadSymbols()
  }, [selectedNode, commits, currentCommitIndex, database])

  if (!selectedNode) {
    return <aside className="w-[280px] border-l border-slate-800 bg-slate-950/70" />
  }

  return (
    <aside className="w-[280px] border-l border-slate-800 bg-slate-950/70 p-3 text-sm text-slate-300">
      <h3 className="mb-2 text-sm font-semibold text-slate-100">{selectedNode.label}</h3>
      <div className="mb-2 rounded bg-slate-900 p-2 text-xs">
        <div><span className="text-slate-500">Kind:</span> {selectedNode.kind}</div>
        {typeof selectedNode.properties.file === 'string' ? <div><span className="text-slate-500">File:</span> {selectedNode.properties.file}</div> : null}
        {typeof selectedNode.properties.path === 'string' ? <div><span className="text-slate-500">Path:</span> {selectedNode.properties.path}</div> : null}
        {typeof selectedNode.properties.lang === 'string' ? <div><span className="text-slate-500">Language:</span> {selectedNode.properties.lang}</div> : null}
        {typeof selectedNode.properties.line_number === 'number' ? <div><span className="text-slate-500">Line:</span> {selectedNode.properties.line_number}</div> : null}
      </div>

      {selectedNode.kind === 'file' ? (
        <div className="mb-2">
          <h4 className="mb-1 text-xs uppercase tracking-wide text-slate-400">Symbols at this commit</h4>
          <ul className="max-h-32 space-y-1 overflow-y-auto rounded bg-slate-900 p-2 text-xs">
            {symbols.length === 0 ? <li className="text-slate-500">No symbols loaded</li> : null}
            {symbols.map((symbol) => (
              <li key={symbol.id} className="truncate">
                {symbol.name} <span className="text-slate-500">({symbol.kind})</span>
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      <h4 className="mb-1 text-xs uppercase tracking-wide text-slate-400">Raw properties</h4>
      <pre className="max-h-64 overflow-auto rounded bg-slate-900 p-2 text-[11px] leading-relaxed">
        {JSON.stringify(selectedNode.properties, null, 2)}
      </pre>
    </aside>
  )
}
