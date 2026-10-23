import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import App from './App'

describe('application shell', () => {
  it('introduces AlumniBridge and keeps unfinished actions unavailable', () => {
    render(<App />)
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Your next chapter starts with')
    expect(screen.getByRole('button', { name: /explore alumni/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: /sign in/i })).toBeDisabled()
  })
})
