export const languageColors: Record<string, string> = {
  go: '#00ADD8',
  typescript: '#3178C6',
  ts: '#3178C6',
  javascript: '#3178C6',
  js: '#3178C6',
  python: '#3776AB',
  py: '#3776AB',
  rust: '#DEA584',
  rs: '#DEA584',
}

export const kindColors: Record<string, string> = {
  function: '#22c55e',
  method: '#06b6d4',
  class: '#a78bfa',
  type: '#f97316',
  struct: '#f59e0b',
  interface: '#f43f5e',
  variable: '#84cc16',
  constant: '#eab308',
  module: '#94a3b8',
  file: '#6b7280',
  directory: '#111827',
}

export function colorForLang(lang?: string): string {
  if (!lang) return '#6B7280'
  return languageColors[lang.toLowerCase()] ?? '#6B7280'
}

export function colorForKind(kind: string): string {
  return kindColors[kind] ?? '#9ca3af'
}
