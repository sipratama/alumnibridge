import { useState } from 'react'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeAll, expect, it } from 'vitest'
import { ConfirmDelete } from './components'

beforeAll(() => {
  Object.defineProperty(HTMLDialogElement.prototype, 'showModal', {
    configurable: true,
    value() {
      this.setAttribute('open', '')
    },
  })
  Object.defineProperty(HTMLDialogElement.prototype, 'close', {
    configurable: true,
    value() {
      this.removeAttribute('open')
    },
  })
})
afterEach(cleanup)
it('returns keyboard focus to the section action after deletion removes the invoking button', () => {
  function History() {
    const [exists, setExists] = useState(true)
    const [confirm, setConfirm] = useState(false)
    return (
      <section aria-label="Experience">
        <button>Add experience</button>
        {exists && (
          <button onClick={() => setConfirm(true)}>Delete experience</button>
        )}
        {confirm && (
          <ConfirmDelete
            title="Remove experience?"
            busy={false}
            error={null}
            onClose={() => setConfirm(false)}
            onConfirm={() => {
              setExists(false)
              setConfirm(false)
            }}
          />
        )}
      </section>
    )
  }
  render(<History />)
  const trigger = screen.getByRole('button', { name: 'Delete experience' })
  trigger.focus()
  fireEvent.click(trigger)
  fireEvent.click(screen.getByRole('button', { name: 'Remove entry' }))
  expect(screen.getByRole('button', { name: 'Add experience' })).toHaveFocus()
})
