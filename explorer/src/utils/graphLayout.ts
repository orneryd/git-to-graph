import {
  forceCenter,
  forceLink,
  forceManyBody,
  forceSimulation,
  type Simulation,
  type SimulationLinkDatum,
  type SimulationNodeDatum,
} from 'd3-force'

export interface LayoutNode extends SimulationNodeDatum {
  id: string
}

export interface LayoutLink extends SimulationLinkDatum<LayoutNode> {
  id: string
  source: string | LayoutNode
  target: string | LayoutNode
  type: string
}

export function createGraphSimulation(
  nodes: LayoutNode[],
  links: LayoutLink[],
  width: number,
  height: number,
): Simulation<LayoutNode, LayoutLink> {
  return forceSimulation(nodes)
    .force(
      'link',
      forceLink<LayoutNode, LayoutLink>(nodes.length > 500 ? links.slice(0, 500) : links)
        .id((d) => d.id)
        .distance(70)
        .strength(0.3),
    )
    .force('charge', forceManyBody().strength(-180))
    .force('center', forceCenter(width / 2, height / 2))
}
