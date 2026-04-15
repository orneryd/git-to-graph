import { select } from 'd3-selection'
import { drag } from 'd3-drag'
import { zoom } from 'd3-zoom'
import { useEffect, useRef } from 'react'
import { useExplorerStore, type ExplorerEdge, type ExplorerNode } from '../store/explorerStore'
import { colorForKind, colorForLang } from '../utils/colors'
import { createGraphSimulation, type LayoutLink, type LayoutNode } from '../utils/graphLayout'

interface DrawNode extends LayoutNode {
  data: ExplorerNode
}

interface DrawLink extends LayoutLink {
  data: ExplorerEdge
}

function hexagonPath(size: number): string {
  return `M ${-size} 0 L ${-size / 2} ${-size} L ${size / 2} ${-size} L ${size} 0 L ${size / 2} ${size} L ${-size / 2} ${size} Z`
}

function diamondPath(size: number): string {
  return `M 0 ${-size} L ${size} 0 L 0 ${size} L ${-size} 0 Z`
}

export function GraphCanvas() {
  const svgRef = useRef<SVGSVGElement | null>(null)
  const nodes = useExplorerStore((s) => s.nodes)
  const edges = useExplorerStore((s) => s.edges)
  const viewMode = useExplorerStore((s) => s.viewMode)
  const selectNode = useExplorerStore((s) => s.selectNode)
  const drillIntoFile = useExplorerStore((s) => s.drillIntoFile)

  useEffect(() => {
    const svgElement = svgRef.current
    if (!svgElement) return

    const width = svgElement.clientWidth || 1000
    const height = svgElement.clientHeight || 700

    const svg = select(svgElement)
    svg.selectAll('*').remove()

    const root = svg.append('g')

    svg.call(
      zoom<SVGSVGElement, unknown>()
        .scaleExtent([0.2, 4])
        .on('zoom', (event) => {
          root.attr('transform', event.transform.toString())
        }),
    )

    const defs = svg.append('defs')
    defs
      .append('marker')
      .attr('id', 'arrow')
      .attr('viewBox', '0 -5 10 10')
      .attr('refX', 14)
      .attr('refY', 0)
      .attr('markerWidth', 6)
      .attr('markerHeight', 6)
      .attr('orient', 'auto')
      .append('path')
      .attr('d', 'M0,-5L10,0L0,5')
      .attr('fill', '#94a3b8')

    const drawNodes: DrawNode[] = nodes.map((n) => ({ id: n.id, data: n }))
    const drawLinks: DrawLink[] = edges.map((e) => ({ id: e.id, source: e.source, target: e.target, type: e.type, data: e }))

    const simulation = createGraphSimulation(drawNodes, drawLinks, width, height)

    const link = root
      .append('g')
      .attr('stroke-opacity', 0.7)
      .selectAll('line')
      .data(drawLinks)
      .join('line')
      .attr('stroke', (d) => (d.type === 'contains' ? '#4b5563' : '#94a3b8'))
      .attr('stroke-width', (d) => (d.type === 'contains' ? 1.2 : 1.6))
      .attr('stroke-dasharray', (d) => (d.type === 'imports' ? '5,3' : null))
      .attr('marker-end', (d) => (d.type === 'calls' || d.type === 'imports' ? 'url(#arrow)' : null))

    const node = root
      .append('g')
      .selectAll<SVGGElement, DrawNode>('g')
      .data(drawNodes)
      .join('g')
      .style('cursor', 'pointer')
      .on('click', (_, d) => {
        selectNode(d.id)
      })
      .on('dblclick', (_, d) => {
        if (viewMode === 'project' && d.data.kind === 'file' && typeof d.data.properties.path === 'string') {
          void drillIntoFile(d.data.properties.path)
        }
      })

    node
      .append('title')
      .text((d) => `${d.data.label} (${d.data.kind})`)

    node.each(function drawShape(d) {
      const g = select(this)

      if (viewMode === 'project') {
        if (d.data.kind === 'directory') {
          g.append('rect')
            .attr('x', -54)
            .attr('y', -16)
            .attr('width', 108)
            .attr('height', 32)
            .attr('rx', 8)
            .attr('fill', '#111827')
            .attr('stroke', '#374151')
        } else {
          g.append('circle')
            .attr('r', 14)
            .attr('fill', colorForLang(d.data.lang))
            .attr('stroke', '#1f2937')
        }
      } else {
        if (d.data.kind === 'class') {
          g.append('rect')
            .attr('x', -24)
            .attr('y', -14)
            .attr('width', 48)
            .attr('height', 28)
            .attr('rx', 8)
            .attr('fill', colorForKind(d.data.kind))
        } else if (d.data.kind === 'method') {
          g.append('path').attr('d', diamondPath(16)).attr('fill', colorForKind(d.data.kind))
        } else if (d.data.kind === 'type' || d.data.kind === 'struct' || d.data.kind === 'interface') {
          g.append('path').attr('d', hexagonPath(16)).attr('fill', colorForKind(d.data.kind))
        } else {
          g.append('circle').attr('r', 16).attr('fill', colorForKind(d.data.kind))
        }
      }

      g.append('text')
        .attr('y', 24)
        .attr('fill', '#e5e7eb')
        .attr('font-size', 10)
        .attr('text-anchor', 'middle')
        .attr('pointer-events', 'none')
        .text(d.data.label.length > 24 ? `${d.data.label.slice(0, 21)}...` : d.data.label)
    })

    node.call(
      drag<SVGGElement, DrawNode>()
        .on('start', (event, d) => {
          if (!event.active) simulation.alphaTarget(0.3).restart()
          d.fx = d.x
          d.fy = d.y
        })
        .on('drag', (event, d) => {
          d.fx = event.x
          d.fy = event.y
        })
        .on('end', (event, d) => {
          if (!event.active) simulation.alphaTarget(0)
          d.fx = null
          d.fy = null
        }),
    )

    simulation.on('tick', () => {
      link
        .attr('x1', (d) => (d.source as DrawNode).x ?? 0)
        .attr('y1', (d) => (d.source as DrawNode).y ?? 0)
        .attr('x2', (d) => (d.target as DrawNode).x ?? 0)
        .attr('y2', (d) => (d.target as DrawNode).y ?? 0)

      node.attr('transform', (d) => `translate(${d.x ?? 0},${d.y ?? 0})`)
    })

    return () => {
      simulation.stop()
    }
  }, [nodes, edges, selectNode, drillIntoFile, viewMode])

  return <svg ref={svgRef} className="h-full w-full" role="img" aria-label="Graph visualization" />
}
