import { useEffect, useRef, useCallback } from 'react'
import * as d3 from 'd3'
import { useNetworkStore } from '../../stores/networkStore'
import type { TopologyData, NetworkDevice } from '../../api/network'

// D3 simulation node extends NetworkDevice with mutable x/y/vx/vy
interface SimNode extends NetworkDevice {
  x?: number
  y?: number
  vx?: number
  vy?: number
  fx?: number | null
  fy?: number | null
}

// D3 simulation link with resolved source/target references
interface SimLink extends d3.SimulationLinkDatum<SimNode> {
  id: number
  source_port: string
  target_port: string
  protocol: string
  bandwidth: string
  last_seen: string
}

interface TopologyGraphProps {
  topology?: TopologyData | null
}

const STATUS_COLORS: Record<string, string> = {
  online: '#2ca07a',
  offline: '#878a99',
}

const NODE_RADIUS_DEFAULT = 18
const NODE_RADIUS_SERVER = 24

function getNodeRadius(device: NetworkDevice): number {
  return device.server_id > 0 ? NODE_RADIUS_SERVER : NODE_RADIUS_DEFAULT
}

function getNodeColor(device: NetworkDevice): string {
  return STATUS_COLORS[device.status] ?? STATUS_COLORS.offline
}

function getDeviceIcon(deviceType: string): string {
  switch (deviceType) {
    case 'switch':
      return 'alt_route'
    case 'router':
      return 'router'
    case 'ap':
      return 'wifi'
    case 'firewall':
      return 'security'
    case 'server':
      return 'dns'
    default:
      return 'device_hub'
  }
}

function getNodeLabel(device: NetworkDevice): string {
  if (device.hostname && device.hostname.trim()) return device.hostname
  return device.ip
}

// Tooltip HTML content
function buildTooltipHTML(device: NetworkDevice): string {
  const statusLabel = device.status === 'online' ? '在线' : '离线'
  const statusColor = getNodeColor(device)
  const snmpLabel = device.snmp_supported ? '支持' : '不支持'
  const rows: [string, string][] = [
    ['IP', device.ip],
    ['状态', `<span style="color:${statusColor};font-weight:600">${statusLabel}</span>`],
    ['类型', device.device_type || '未知'],
    ['厂商', device.vendor || '—'],
    ['型号', device.model || '—'],
    ['SNMP', snmpLabel],
  ]
  if (device.hostname) rows.splice(1, 0, ['主机名', device.hostname])
  const rowsHTML = rows
    .map(
      ([k, v]) =>
        `<tr><td style="color:#8b949e;padding:2px 8px 2px 0;white-space:nowrap">${k}</td><td style="color:#e6edf3">${v}</td></tr>`
    )
    .join('')
  return `
    <div style="font-family:inherit;font-size:12px;min-width:160px">
      <table style="border-collapse:collapse;width:100%">${rowsHTML}</table>
    </div>
  `
}

export default function TopologyGraph({ topology: propTopology }: TopologyGraphProps) {
  const storeTopology = useNetworkStore((s) => s.topology)
  const topology = propTopology !== undefined ? propTopology : storeTopology

  const svgRef = useRef<SVGSVGElement>(null)
  const tooltipRef = useRef<HTMLDivElement>(null)
  const simulationRef = useRef<d3.Simulation<SimNode, SimLink> | null>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  // Stable tooltip show/hide helpers using refs
  const showTooltip = useCallback((event: MouseEvent, device: NetworkDevice) => {
    const tooltip = tooltipRef.current
    if (!tooltip) return
    tooltip.innerHTML = buildTooltipHTML(device)
    tooltip.style.display = 'block'
    const container = containerRef.current
    if (!container) return
    const rect = container.getBoundingClientRect()
    let x = event.clientX - rect.left + 12
    let y = event.clientY - rect.top - 8
    // Prevent overflow on right edge
    if (x + 200 > rect.width) x = event.clientX - rect.left - 200 - 12
    tooltip.style.left = `${x}px`
    tooltip.style.top = `${y}px`
  }, [])

  const hideTooltip = useCallback(() => {
    const tooltip = tooltipRef.current
    if (tooltip) tooltip.style.display = 'none'
  }, [])

  const moveTooltip = useCallback((event: MouseEvent) => {
    const tooltip = tooltipRef.current
    if (!tooltip || tooltip.style.display === 'none') return
    const container = containerRef.current
    if (!container) return
    const rect = container.getBoundingClientRect()
    let x = event.clientX - rect.left + 12
    let y = event.clientY - rect.top - 8
    if (x + 200 > rect.width) x = event.clientX - rect.left - 200 - 12
    tooltip.style.left = `${x}px`
    tooltip.style.top = `${y}px`
  }, [])

  useEffect(() => {
    const svgEl = svgRef.current
    if (!svgEl) return

    // Stop and cleanup previous simulation
    if (simulationRef.current) {
      simulationRef.current.stop()
      simulationRef.current = null
    }

    const svg = d3.select(svgEl)
    svg.selectAll('*').remove()

    const hasData = topology && topology.nodes && topology.nodes.length > 0
    if (!hasData) return

    const width = svgEl.clientWidth || 800
    const height = svgEl.clientHeight || 500

    // Root group for zoom/pan
    const rootG = svg.append('g').attr('class', 'root')

    // --- Zoom behavior ---
    const zoomBehavior = d3
      .zoom<SVGSVGElement, unknown>()
      .scaleExtent([0.2, 4])
      .on('zoom', (event: d3.D3ZoomEvent<SVGSVGElement, unknown>) => {
        rootG.attr('transform', event.transform.toString())
      })
    svg.call(zoomBehavior)

    // Prevent double-click zoom
    svg.on('dblclick.zoom', null)

    // --- Build simulation nodes and links ---
    const nodes: SimNode[] = topology!.nodes.map((d) => ({ ...d }))
    const nodeById = new Map<number, SimNode>(nodes.map((n) => [n.id, n]))

    const links: SimLink[] = topology!.edges
      .filter((e) => nodeById.has(e.source_id) && nodeById.has(e.target_id))
      .map((e) => ({
        ...e,
        source: nodeById.get(e.source_id)!,
        target: nodeById.get(e.target_id)!,
      }))

    // --- Simulation ---
    const simulation = d3
      .forceSimulation<SimNode, SimLink>(nodes)
      .force(
        'link',
        d3
          .forceLink<SimNode, SimLink>(links)
          .id((d) => d.id)
          .distance(100)
      )
      .force('charge', d3.forceManyBody<SimNode>().strength(-200))
      .force('center', d3.forceCenter(width / 2, height / 2))
      .force('collide', d3.forceCollide<SimNode>().radius(40))

    simulationRef.current = simulation

    // --- Arrow marker (optional, for future directed edges) ---
    svg
      .append('defs')
      .append('marker')
      .attr('id', 'arrowhead')
      .attr('viewBox', '-0 -5 10 10')
      .attr('refX', 28)
      .attr('refY', 0)
      .attr('orient', 'auto')
      .attr('markerWidth', 6)
      .attr('markerHeight', 6)
      .append('path')
      .attr('d', 'M 0,-5 L 10,0 L 0,5')
      .attr('fill', '#8b949e')

    // --- Link lines ---
    const linkG = rootG.append('g').attr('class', 'links')
    const linkEls = linkG
      .selectAll<SVGLineElement, SimLink>('line')
      .data(links)
      .join('line')
      .attr('stroke', '#e9ebec')
      .attr('stroke-width', 1.5)
      .attr('stroke-dasharray', (d) => {
        const srcNode = nodeById.get((d as unknown as { source_id: number }).source_id)
        const tgtNode = nodeById.get((d as unknown as { target_id: number }).target_id)
        const offline =
          (srcNode && srcNode.status !== 'online') ||
          (tgtNode && tgtNode.status !== 'online')
        return offline ? '5,4' : 'none'
      })

    // --- Node groups ---
    const nodeG = rootG.append('g').attr('class', 'nodes')

    // Drag behavior
    const drag = d3
      .drag<SVGGElement, SimNode>()
      .on('start', (event: d3.D3DragEvent<SVGGElement, SimNode, SimNode>, d) => {
        if (!event.active) simulation.alphaTarget(0.3).restart()
        d.fx = d.x
        d.fy = d.y
        hideTooltip()
      })
      .on('drag', (event: d3.D3DragEvent<SVGGElement, SimNode, SimNode>, d) => {
        d.fx = event.x
        d.fy = event.y
      })
      .on('end', (event: d3.D3DragEvent<SVGGElement, SimNode, SimNode>, d) => {
        if (!event.active) simulation.alphaTarget(0)
        d.fx = null
        d.fy = null
      })

    const nodeEls = nodeG
      .selectAll<SVGGElement, SimNode>('g.node')
      .data(nodes, (d) => String(d.id))
      .join('g')
      .attr('class', 'node')
      .style('cursor', 'grab')
      .call(drag)

    // Shadow/glow filter
    const defs = svg.select('defs')
    const filter = defs
      .append('filter')
      .attr('id', 'node-shadow')
      .attr('x', '-30%')
      .attr('y', '-30%')
      .attr('width', '160%')
      .attr('height', '160%')
    filter
      .append('feDropShadow')
      .attr('dx', 0)
      .attr('dy', 2)
      .attr('stdDeviation', 3)
      .attr('flood-color', 'rgba(0,0,0,0.18)')

    // Circle background
    nodeEls
      .append('circle')
      .attr('r', (d) => getNodeRadius(d))
      .attr('fill', (d) => getNodeColor(d))
      .attr('stroke', '#fff')
      .attr('stroke-width', 2)
      .attr('filter', 'url(#node-shadow)')

    // Material Symbols icon text (uses loaded font)
    nodeEls
      .append('text')
      .attr('class', 'material-symbols-outlined')
      .attr('text-anchor', 'middle')
      .attr('dominant-baseline', 'central')
      .attr('fill', '#fff')
      .attr('font-size', (d) => (getNodeRadius(d) === NODE_RADIUS_SERVER ? '16px' : '13px'))
      .attr('pointer-events', 'none')
      .attr('y', 0)
      .text((d) => getDeviceIcon(d.device_type))

    // Label below node
    nodeEls
      .append('text')
      .attr('text-anchor', 'middle')
      .attr('dy', (d) => getNodeRadius(d) + 13)
      .attr('fill', '#495057')
      .attr('font-size', '10px')
      .attr('font-weight', '500')
      .attr('pointer-events', 'none')
      .text((d) => {
        const label = getNodeLabel(d)
        return label.length > 16 ? label.slice(0, 14) + '…' : label
      })

    // Tooltip interactions
    nodeEls
      .on('mouseenter', function (event: MouseEvent, d: SimNode) {
        d3.select(this)
          .select('circle')
          .transition()
          .duration(120)
          .attr('r', getNodeRadius(d) + 3)
          .attr('stroke-width', 3)
        showTooltip(event, d)
      })
      .on('mousemove', function (event: MouseEvent) {
        moveTooltip(event)
      })
      .on('mouseleave', function (_event: MouseEvent, d: SimNode) {
        d3.select(this)
          .select('circle')
          .transition()
          .duration(120)
          .attr('r', getNodeRadius(d))
          .attr('stroke-width', 2)
        hideTooltip()
      })

    // --- Tick handler ---
    simulation.on('tick', () => {
      linkEls
        .attr('x1', (d) => (d.source as SimNode).x ?? 0)
        .attr('y1', (d) => (d.source as SimNode).y ?? 0)
        .attr('x2', (d) => (d.target as SimNode).x ?? 0)
        .attr('y2', (d) => (d.target as SimNode).y ?? 0)

      nodeEls.attr('transform', (d) => `translate(${d.x ?? 0},${d.y ?? 0})`)
    })

    // Cleanup
    return () => {
      simulation.stop()
      simulationRef.current = null
      hideTooltip()
    }
  }, [topology, showTooltip, hideTooltip, moveTooltip])

  const hasData = topology && topology.nodes && topology.nodes.length > 0

  return (
    <div
      ref={containerRef}
      className="relative w-full rounded-xl overflow-hidden"
      style={{ height: 500, background: '#f8f9fa' }}
    >
      {!hasData ? (
        <div className="flex flex-col items-center justify-center h-full text-[#6c757d]">
          <span className="material-symbols-outlined text-5xl mb-3 text-[#adb5bd]">
            device_hub
          </span>
          <p className="text-sm font-medium">暂无拓扑数据，请先扫描网段</p>
        </div>
      ) : (
        <svg
          ref={svgRef}
          className="w-full h-full"
          aria-label="网络拓扑图"
          role="img"
        />
      )}

      {/* Tooltip overlay */}
      <div
        ref={tooltipRef}
        style={{
          display: 'none',
          position: 'absolute',
          pointerEvents: 'none',
          background: '#161b22',
          border: '1px solid #30363d',
          borderRadius: 8,
          padding: '8px 12px',
          zIndex: 50,
          boxShadow: '0 4px 16px rgba(0,0,0,0.24)',
          maxWidth: 260,
        }}
      />

      {/* Zoom hint */}
      {hasData && (
        <div
          style={{
            position: 'absolute',
            bottom: 10,
            right: 14,
            fontSize: 11,
            color: '#adb5bd',
            pointerEvents: 'none',
            userSelect: 'none',
          }}
        >
          滚轮缩放 · 拖拽移动
        </div>
      )}
    </div>
  )
}
