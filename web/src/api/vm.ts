export interface VMQueryResult {
  metric: Record<string, string>
  values: [number, string][]
}

interface VMResponse {
  status: string
  data: {
    resultType: string
    result: VMQueryResult[]
  }
}

export async function queryRange(
  query: string,
  start: number,
  end: number,
  step: number,
  signal?: AbortSignal
): Promise<VMQueryResult[]> {
  const params = new URLSearchParams({
    query,
    start: start.toString(),
    end: end.toString(),
    step: step.toString(),
  })
  const res = await fetch(`/vm/api/v1/query_range?${params}`, { signal })
  if (!res.ok) throw new Error(`VM query failed: ${res.status}`)
  const data: VMResponse = await res.json()
  if (data.status !== 'success') throw new Error(`VM query error: ${data.status}`)
  return data.data.result
}
