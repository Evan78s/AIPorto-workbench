import { useCallback, useEffect, useState } from 'react'

export function useFetch(loader, deps = []) {
  const [data, setData] = useState(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const reload = useCallback(() => {
    setLoading(true)
    setError('')
    loader()
      .then(setData)
      .catch((e) => setError(e.message || String(e)))
      .finally(() => setLoading(false))
  }, deps)

  useEffect(() => { reload() }, [reload])
  return { data, error, loading, reload }
}
