import { http } from './request'

export interface MetricPoint {
  ts: number
  value: number
}

export interface MetricSeries {
  metric: string
  unit: string
  labels?: Record<string, string>
  points: MetricPoint[]
}

export interface LogEntry {
  timestamp: number
  level: string
  service: string
  message: string
  trace_id?: string
  labels?: Record<string, string>
  ref?: string
}

export interface TraceSpan {
  trace_id: string
  span_id: string
  service: string
  operation: string
  start_time: number
  duration_ms: number
  status: string
  error: boolean
  error_summary?: string
}

export interface TopologyNode {
  node_id: string
  name: string
  type: string
  error_rate?: number
  p95_ms?: number
}

export interface TopologyEdge {
  from: string
  to: string
  call_count: number
  error_rate?: number
}

export interface TopologySnapshot {
  nodes: TopologyNode[]
  edges: TopologyEdge[]
}

export interface MetricQueryInput {
  account_id: string
  provider?: string
  region?: string
  namespace?: string
  metric: string
  dimensions?: Record<string, string>
  from: number
  to: number
  period?: number
  aggregator?: string
}

export interface LogSearchInput {
  account_id: string
  provider?: string
  service?: string
  resource_id?: string
  keyword?: string
  trace_id?: string
  from: number
  to: number
  limit?: number
}

export interface TraceQueryInput {
  account_id: string
  provider?: string
  service?: string
  operation?: string
  trace_id?: string
  error_only?: boolean
  min_latency_ms?: number
  from: number
  to: number
  limit?: number
}

export interface TopologyQuery {
  account_id: string
  provider?: string
  application_id?: string
  from: number
  to: number
}

export interface MetricQueryResult {
  series: MetricSeries[]
  evidence_id: string
}

export interface LogSearchResult {
  entries: LogEntry[]
  evidence_id: string
}

export interface TraceQueryResult {
  spans: TraceSpan[]
  evidence_id: string
}

export interface TopologyQueryResult {
  topology: TopologySnapshot
  evidence_id: string
}

export function queryMetrics(input: MetricQueryInput) {
  return http<MetricQueryResult>({
    url: '/api/observability/metrics/query',
    method: 'post',
    data: input
  })
}

export function searchLogs(input: LogSearchInput) {
  return http<LogSearchResult>({
    url: '/api/observability/logs/search',
    method: 'post',
    data: input
  })
}

export function queryTraces(input: TraceQueryInput) {
  return http<TraceQueryResult>({
    url: '/api/observability/traces/query',
    method: 'post',
    data: input
  })
}

export function queryTopology(query: TopologyQuery) {
  return http<TopologyQueryResult>({
    url: '/api/observability/topology',
    method: 'get',
    params: query
  })
}
