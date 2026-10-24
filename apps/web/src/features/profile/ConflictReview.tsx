import { useState } from 'react'
import { Button } from '@/components/ui/button'

export type ConflictField = {
  key: string
  label: string
  original: unknown
  current: unknown
  draft: unknown
}

function valueText(value: unknown): string {
  if (value === null || value === undefined || value === '') return '(empty)'
  if (typeof value === 'boolean') return value ? 'On' : 'Off'
  return String(value)
}

// This view never changes a validator. The editor owns the explicit rebase
// action and leaves its original draft intact until that action is selected.
export function ConflictReview({
  fields,
  onApply,
}: {
  fields: ConflictField[]
  onApply: (selected: Set<string>) => void
}) {
  const [selected, setSelected] = useState(() => new Set(fields.map((field) => field.key)))
  return (
    <section className="conflict-review" aria-label="Review current server values">
      <h3>Review changes before saving again</h3>
      <p>Your draft is preserved. Compare each value, then choose which of your edits to carry onto the latest saved version. You will review the resulting draft before saving.</p>
      {fields.length === 0 ? <p>No changed fields remain in this draft.</p> : fields.map((field) => (
        <div className="conflict-field" key={field.key}>
          <label>
            <input
              type="checkbox"
              checked={selected.has(field.key)}
              onChange={(event) => {
                const next = new Set(selected)
                if (event.target.checked) next.add(field.key)
                else next.delete(field.key)
                setSelected(next)
              }}
            />
            Keep my edit for {field.label}
          </label>
          <dl>
            <div><dt>When I started</dt><dd>{valueText(field.original)}</dd></div>
            <div><dt>Currently saved</dt><dd>{valueText(field.current)}</dd></div>
            <div><dt>My draft</dt><dd>{valueText(field.draft)}</dd></div>
          </dl>
        </div>
      ))}
      <Button type="button" variant="outline" onClick={() => onApply(selected)}>
        Apply selected edits to latest draft
      </Button>
    </section>
  )
}
