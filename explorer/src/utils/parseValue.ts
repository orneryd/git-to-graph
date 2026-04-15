export type JsonMap = Record<string, unknown>

export function parseValue(value: unknown): JsonMap {
  if (typeof value === 'string') {
    try {
      return JSON.parse(value) as JsonMap
    } catch {
      return {}
    }
  }

  if (value && typeof value === 'object') {
    return value as JsonMap
  }

  return {}
}

export function asString(value: unknown): string | null {
  return typeof value === 'string' ? value : null
}

export function extractPathOrId(parsed: JsonMap): string | null {
  const path = asString(parsed.path)
  if (path) return path

  const id = asString(parsed.id)
  if (id) return id

  return null
}
