// hooks.ts — minimal data-fetching hook.

import { useEffect, useRef, useState } from 'react'
import { APIError, fetchJSON } from './api'

export type FetchState<T> = {
  data: T | null
  loading: boolean
  error: APIError | Error | null
  refetch: () => void
}

export function useFetch<T>(path: string | null): FetchState<T> {
  const [data, setData] = useState<T | null>(null)
  const [loading, setLoading] = useState(path !== null)
  const [error, setError] = useState<APIError | Error | null>(null)
  const [tick, setTick] = useState(0)
  const cancelledRef = useRef(false)

  useEffect(() => {
    cancelledRef.current = false
    if (path === null) {
      setLoading(false)
      return
    }
    setLoading(true)
    fetchJSON<T>(path)
      .then((d) => {
        if (cancelledRef.current) return
        setData(d)
        setError(null)
      })
      .catch((e) => {
        if (cancelledRef.current) return
        setError(e)
      })
      .finally(() => {
        if (cancelledRef.current) return
        setLoading(false)
      })
    return () => {
      cancelledRef.current = true
    }
  }, [path, tick])

  return { data, loading, error, refetch: () => setTick((t) => t + 1) }
}
