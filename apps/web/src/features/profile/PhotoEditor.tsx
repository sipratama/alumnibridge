import { useEffect, useRef, useState } from 'react'
import { Camera, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { apiRequest } from '@/shared/api/client'
import { ownerPath, useOwnerMutation } from './api'
import { Avatar, ConfirmDelete, Feedback } from './components'
import { validatePhoto } from './validation'

export function PhotoEditor({
  name,
  src,
  error,
}: {
  name: string
  src?: string
  error?: unknown
}) {
  const [selection, setSelection] = useState<{ file: File; url: string }>()
  const file = selection?.file
  const preview = selection?.url
  const [validation, setValidation] = useState('')
  const [message, setMessage] = useState('')
  const [confirm, setConfirm] = useState(false)
  const input = useRef<HTMLInputElement>(null)
  useEffect(() => {
    return () => {
      if (preview) URL.revokeObjectURL(preview)
    }
  }, [preview])
  const upload = useOwnerMutation(async (image: File, token) =>
    apiRequest(`${ownerPath}/profile/photo`, {
      method: 'POST',
      body: image,
      headers: {
        Authorization: `Bearer ${await token()}`,
        'Content-Type': image.type,
      },
    }),
  )
  const remove = useOwnerMutation(async (_: void, token) =>
    apiRequest<void>(`${ownerPath}/profile/photo`, {
      method: 'DELETE',
      headers: { Authorization: `Bearer ${await token()}` },
    }),
  )
  const busy = upload.isPending || remove.isPending
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
          setSelection(
            issue
              ? undefined
              : { file: selected, url: URL.createObjectURL(selected) },
          )
        }}
      />
      {file && (
        <div className="photo-preview-actions">
          <p>Preview only. Save to upload this photo.</p>
          <Button
            disabled={busy}
            onClick={() => {
              void upload
                .mutateAsync(file)
                .then(() => {
                  setSelection(undefined)
                  setMessage('Photo saved.')
                })
                .catch(() => {})
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
      {confirm && (
        <ConfirmDelete
          title="Remove profile photo?"
          busy={remove.isPending}
          error={remove.error}
          onClose={() => setConfirm(false)}
          onConfirm={() => {
            void remove
              .mutateAsync()
              .then(() => {
                setConfirm(false)
                setSelection(undefined)
                setMessage('Photo removed.')
              })
              .catch(() => {})
          }}
        />
      )}
    </div>
  )
}
