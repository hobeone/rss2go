import { render, screen, fireEvent } from '@testing-library/svelte'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import LoginPanel from './LoginPanel.svelte'

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
    const fetchMock = vi.fn().mockResolvedValue({
      status: 200,
      ok: true,
      text: async () => 'OK'
    })
    vi.stubGlobal('fetch', fetchMock)

    render(LoginPanel, { onLoginSuccess: onLoginSuccessMock })

    const passwordInput = screen.getByPlaceholderText('••••••••')
    const submitBtn = screen.getByRole('button', { name: 'Unlock Panel' })

    await fireEvent.input(passwordInput, { target: { value: 'correct-password' } })
    await fireEvent.click(submitBtn)

    expect(fetchMock).toHaveBeenCalledWith('/api/v1/login', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ password: 'correct-password' })
    }))
    expect(onLoginSuccessMock).toHaveBeenCalled()
    expect(passwordInput).toHaveValue('')
  })

  it('displays error message on invalid credentials (HTTP 401)', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      status: 401,
      ok: false,
      text: async () => 'Invalid credentials'
    })
    vi.stubGlobal('fetch', fetchMock)

    render(LoginPanel, { onLoginSuccess: onLoginSuccessMock })

    const passwordInput = screen.getByPlaceholderText('••••••••')
    const submitBtn = screen.getByRole('button', { name: 'Unlock Panel' })

    await fireEvent.input(passwordInput, { target: { value: 'wrong-password' } })
    await fireEvent.click(submitBtn)

    expect(onLoginSuccessMock).not.toHaveBeenCalled()
    expect(screen.getByText('Invalid credentials')).toBeInTheDocument()
  })

  it('displays network error message on fetch error', async () => {
    const fetchMock = vi.fn().mockRejectedValue(new Error('Connection timed out'))
    vi.stubGlobal('fetch', fetchMock)

    render(LoginPanel, { onLoginSuccess: onLoginSuccessMock })

    const passwordInput = screen.getByPlaceholderText('••••••••')
    const submitBtn = screen.getByRole('button', { name: 'Unlock Panel' })

    await fireEvent.input(passwordInput, { target: { value: 'some-password' } })
    await fireEvent.click(submitBtn)

    expect(onLoginSuccessMock).not.toHaveBeenCalled()
    expect(screen.getByText('Connection timed out')).toBeInTheDocument()
  })
})
