import { render, screen, fireEvent } from '@testing-library/svelte'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import LoginPanel from './LoginPanel.svelte'
import * as api from '../api'

vi.mock('../api', () => ({
  login: vi.fn()
}))

describe('LoginPanel', () => {
  const onLoginSuccessMock = vi.fn()

  beforeEach(() => {
    vi.restoreAllMocks()
    onLoginSuccessMock.mockClear()
  })

  it('renders login form correctly', () => {
    render(LoginPanel, { onLoginSuccess: onLoginSuccessMock })

    expect(screen.getByText('rss2go aggregate')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('••••••••')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Unlock Panel' })).toBeInTheDocument()
  })

  it('calls onLoginSuccess on successful login (HTTP 200)', async () => {
    vi.mocked(api.login).mockResolvedValue(true)

    render(LoginPanel, { onLoginSuccess: onLoginSuccessMock })

    const passwordInput = screen.getByPlaceholderText('••••••••')
    const submitBtn = screen.getByRole('button', { name: 'Unlock Panel' })

    await fireEvent.input(passwordInput, { target: { value: 'correct-password' } })
    await fireEvent.click(submitBtn)

    expect(api.login).toHaveBeenCalledWith('correct-password')
    expect(onLoginSuccessMock).toHaveBeenCalled()
    expect(passwordInput).toHaveValue('')
  })

  it('displays error message on invalid credentials (HTTP 401)', async () => {
    vi.mocked(api.login).mockResolvedValue(false)

    render(LoginPanel, { onLoginSuccess: onLoginSuccessMock })

    const passwordInput = screen.getByPlaceholderText('••••••••')
    const submitBtn = screen.getByRole('button', { name: 'Unlock Panel' })

    await fireEvent.input(passwordInput, { target: { value: 'wrong-password' } })
    await fireEvent.click(submitBtn)

    expect(onLoginSuccessMock).not.toHaveBeenCalled()
    expect(screen.getByText('Invalid credentials')).toBeInTheDocument()
  })

  it('displays network error message on fetch error', async () => {
    vi.mocked(api.login).mockRejectedValue(new Error('Connection timed out'))

    render(LoginPanel, { onLoginSuccess: onLoginSuccessMock })

    const passwordInput = screen.getByPlaceholderText('••••••••')
    const submitBtn = screen.getByRole('button', { name: 'Unlock Panel' })

    await fireEvent.input(passwordInput, { target: { value: 'some-password' } })
    await fireEvent.click(submitBtn)

    expect(onLoginSuccessMock).not.toHaveBeenCalled()
    expect(screen.getByText('Connection timed out')).toBeInTheDocument()
  })
})
