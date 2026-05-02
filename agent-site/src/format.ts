// format.ts — small display formatters.

export function formatBytes(n: number): string {
  if (!Number.isFinite(n)) return '—'
  if (n < 0) return '-' + formatBytes(-n)
  if (n < 1024) return `${n}b`
  const k = n / 1024
  if (k < 1024) return `${Math.round(k)}k`
  const m = k / 1024
  if (m < 1024) return `${m.toFixed(1)}mb`
  const g = m / 1024
  if (g < 1024) return `${g.toFixed(1)}gb`
  return `${(g / 1024).toFixed(1)}tb`
}

// for byte deltas — keeps an explicit + sign for positives.
export function formatBytesDelta(n: number): string {
  if (!Number.isFinite(n)) return '—'
  if (n === 0) return '0'
  return (n > 0 ? '+' : '') + formatBytes(n)
}
