import { useEffect, useRef, useState } from 'react'
import { Camera, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { ApiError, apiRequest } from '@/shared/api/client'
import { ownerPath, useOwnerMutation, type Profile } from './api'
import { Avatar, ConfirmDelete, Feedback } from './components'
import { validatePhoto } from './validation'

export function PhotoEditor({
  name,
  src,
  error,
  etag,
  reloadProfile,
}: {
  name: string
  src?: string
  error?: unknown
  etag?: string
  reloadProfile?: () => Promise<Profile | undefined>
}) {
  const [selection, setSelection] = useState<{ file: File; url: string; etag?: string }>()
  const file = selection?.file
  const preview = selection?.url
  const [validation, setValidation] = useState('')
  const [message, setMessage] = useState('')
  const [confirm, setConfirm] = useState(false)
  const [deleteETag, setDeleteETag] = useState<string>()
  const [latest, setLatest] = useState<Profile>()
  const [reloadError, setReloadError] = useState(false)
  const input = useRef<HTMLInputElement>(null)
  useEffect(() => {
    return () => {
      if (preview) URL.revokeObjectURL(preview)
    }
  }, [preview])
  const upload = useOwnerMutation(async (image: { file: File; etag?: string }, token) =>
    apiRequest(`${ownerPath}/profile/photo`, {
      method: 'POST',
      body: image.file,
      headers: {
        Authorization: `Bearer ${await token()}`,
        'Content-Type': image.file.type,
        ...(image.etag ? { 'If-Match': image.etag } : {}),
      },
    }),
  )
  const remove = useOwnerMutation(async (expected: string | undefined, token) =>
    apiRequest<void>(`${ownerPath}/profile/photo`, {
      method: 'DELETE',
      headers: { Authorization: `Bearer ${await token()}`, ...(expected ? { 'If-Match': expected } : {}) },
    }),
  )
  const busy = upload.isPending || remove.isPending
  async function loadLatest() {
    if (!reloadProfile) return
    try {
      setLatest(await reloadProfile())
      setReloadError(false)
    } catch {
      setReloadError(true)
    }
  }
  return (
    <div className="photo-editor">
      <div className="photo-row">
        <Avatar name={name} src={preview || src} />
        <div>
          <strong>Profile photo</strong>
          <p className="muted">JPEG, PNG or WebP · up to 5 MiB</p>
          <div className="photo-actions">
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => input.current?.click()}
            >
              <Camera size={16} aria-hidden="true" />
              {src ? 'Replace photo' : 'Choose photo'}
            </Button>
            {src && (
              <button
                className="icon-action"
                disabled={busy}
                aria-label="Delete profile photo"
                onClick={() => {
                  remove.reset()
                  setDeleteETag(etag)
                  setLatest(undefined)
                  setConfirm(true)
                }}
              >
                <Trash2 size={18} />
              </button>
            )}
          </div>
        </div>
      </div>
      <input
        ref={input}
        className="sr-only"
        type="file"
        tabIndex={-1}
        aria-label="Profile photo file"
        accept="image/jpeg,image/png,image/webp"
        onChange={(event) => {
          const selected = event.target.files?.[0]
          event.target.value = ''
          if (!selected) return
          const issue = validatePhoto(selected)
          setValidation(issue || '')
          setMessage('')
          upload.reset()
          setLatest(undefined)
          setSelection(
            issue
              ? undefined
              : { file: selected, url: URL.createObjectURL(selected), etag },
          )
        }}
      />
      {file && (
        <div className="photo-preview-actions">
          <p>Preview only. Save to upload this photo.</p>
          <Button
            disabled={busy || !!latest}
            onClick={() => {
              void upload
                .mutateAsync({ file, etag: selection?.etag })
                .then(() => {
                  setSelection(undefined)
                  setMessage('Photo saved.')
                })
                .catch((caught) => { if (caught instanceof ApiError && caught.status === 412) void loadLatest() })
            }}
          >
            {upload.isPending ? 'Uploading…' : 'Save photo'}
          </Button>
          <Button
            variant="outline"
            disabled={busy}
            onClick={() => {
              setSelection(undefined)
              upload.reset()
            }}
          >
            Cancel
          </Button>
        </div>
      )}
      {validation && (
        <p role="alert" className="profile-error">
          {validation}
        </p>
      )}
      <Feedback error={upload.error || error} message={message} />
      {latest && file && <div className="conflict-review" role="alert">
        <p>The profile changed while you prepared this photo. Review the current profile before retrying. Your selected photo remains in preview.</p>
        <p>Currently saved profile: {latest.display_name} · {latest.headline || '(no headline)'}</p>
        <Button type="button" variant="outline" onClick={() => { setSelection({ ...selection!, etag: latest.__etag }); setLatest(undefined); upload.reset() }}>Use current profile revision for this photo</Button>
      </div>}
      {reloadError && <p role="alert">Current profile could not be loaded. Your photo selection is preserved.</p>}
      {file && upload.error instanceof ApiError && upload.error.status === 412 && <Button type="button" variant="outline" onClick={() => void loadLatest()}>Reload current profile</Button>}
      {confirm && (
        <ConfirmDelete
          title="Remove profile photo?"
          busy={remove.isPending}
          error={remove.error}
          onClose={() => setConfirm(false)}
          onConfirm={() => {
            void remove
              .mutateAsync(deleteETag)
              .then(() => {
                setConfirm(false)
                setSelection(undefined)
                setMessage('Photo removed.')
              })
              .catch((caught) => { if (caught instanceof ApiError && caught.status === 412) void loadLatest() })
          }}
          confirmDisabled={!!latest}
        >
          {latest && <div className="conflict-review" role="alert">
            <p>The profile changed. Current profile: {latest.display_name} · {latest.headline || '(no headline)'}. Review before deleting the current photo.</p>
            <Button type="button" variant="outline" onClick={() => { setDeleteETag(latest.__etag); setLatest(undefined); remove.reset() }}>Use current profile revision for deletion</Button>
          </div>}
          {reloadError && <p role="alert">Current profile could not be loaded. Deletion remains paused.</p>}
          {remove.error instanceof ApiError && remove.error.status === 412 && <Button type="button" variant="outline" onClick={() => void loadLatest()}>Reload current profile</Button>}
        </ConfirmDelete>
      )}
    </div>
  )
}
