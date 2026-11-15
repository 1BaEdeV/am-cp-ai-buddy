import { useCallback, useEffect, useRef, useState } from 'react'

// При смене экрана или пользователя запоздалый ответ не заменяет новые данные.
export function useResource<T>(load: () => Promise<T>, interval = 0) {
  const [data, setData] = useState<T>()
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const generation = useRef(0)
  const inFlight = useRef<{ load: () => Promise<T>; promise: Promise<void> } | null>(null)
  const invalidate = useCallback(() => {
    generation.current++
    inFlight.current = null
  }, [])
  const refresh = useCallback(() => {
    if (inFlight.current?.load === load) return inFlight.current.promise
    const current = ++generation.current
    const promise = Promise.resolve()
      .then(load)
      .then((value) => {
        if (current === generation.current) {
          setData(value)
          setError('')
        }
      })
      .catch((error) => {
        if (current === generation.current)
          setError(error instanceof Error ? error.message : 'Не удалось загрузить данные.')
      })
      .finally(() => {
        if (current === generation.current) setLoading(false)
        if (inFlight.current?.promise === promise) inFlight.current = null
      })
    inFlight.current = { load, promise }
    return promise
  }, [load])
  useEffect(() => {
    void refresh()
    const timer = interval
      ? window.setInterval(() => {
          if (!document.hidden) void refresh()
        }, interval)
      : undefined
    const onFocus = () => {
      void refresh()
    }
    window.addEventListener('focus', onFocus)
    return () => {
      invalidate()
      clearInterval(timer)
      window.removeEventListener('focus', onFocus)
    }
  }, [refresh, interval, invalidate])
  return { data, error, loading, refresh }
}
