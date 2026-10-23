import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import App from './App'

vi.mock('./app/authContext', () => ({ useAuth: () => ({ status: 'anonymous', login: vi.fn(), register: vi.fn(), logout: vi.fn() }) }))

describe('application shell', () => {
  it('introduces AlumniBridge and offers sign-in without showing unfinished directory access', () => {
    render(<MemoryRouter><App /></MemoryRouter>)
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Your next chapter starts with')
    expect(screen.getByRole('button', { name: /join the community/i })).toBeEnabled()
    expect(screen.getByRole('button', { name: /sign in/i })).toBeEnabled()
  })
})
