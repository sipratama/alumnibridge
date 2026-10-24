import { useEffect, useState } from 'react'
import { useAuth } from '@/app/authContext'
import { photoBlob } from './api'

export function usePhoto(
  path: string | undefined,
  revision: number,
  owner = false,
) {
  const { getToken, user } = useAuth()
  const [result, setResult] = useState<{
    path: string
    revision: number
    actor?: string
    url?: string
    error?: unknown
  }>()
  const actor = owner ? user?.id : undefined
  useEffect(() => {
    if (!path) return
    const controller = new AbortController()
    let url: string | undefined
    void photoBlob(path, controller.signal, owner ? getToken : undefined)
      .then((blob) => {
        if (controller.signal.aborted) return
        if (blob) url = URL.createObjectURL(blob)
        setResult({ path, revision, actor, url })
      })
      .catch((error) => {
        if (!controller.signal.aborted)
          setResult({ path, revision, actor, error })
      })
    return () => {
      controller.abort()
      if (url) URL.revokeObjectURL(url)
    }
  }, [path, revision, owner, actor, getToken])
  return result &&
    result.path === path &&
    result.revision === revision &&
    result.actor === actor
    ? result
    : undefined
}
