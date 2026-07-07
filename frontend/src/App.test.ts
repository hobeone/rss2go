import { render, screen, fireEvent } from '@testing-library/svelte'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import App from './App.svelte'
import * as api from './lib/api'

vi.mock('./lib/api', () => ({
  fetchStats: vi.fn(),
  fetchFeeds: vi.fn(),
  fetchOutbox: vi.fn(),
  logout: vi.fn(),
  fetchUsers: vi.fn()
}))

class MockEventSource {
  onmessage = null
  onerror = null
  close = vi.fn()
}

describe('App Component Layout Shell', () => {
  const mockStats = {
    total_feeds: 5,
    total_users: 10,
    outbox_pending: 1,
    outbox_failed: 0,
    outbox_delivered: 20,
    mailer_mode: 'smtp'
  }
  const mockFeeds = [
    { id: 1, title: 'Tech Blog', url: 'https://tech.com/feed' }
  ]

  beforeEach(() => {
    vi.restoreAllMocks()
    localStorage.clear()
    vi.stubGlobal('EventSource', MockEventSource)
    vi.mocked(api.fetchStats).mockResolvedValue(mockStats)
    vi.mocked(api.fetchFeeds).mockResolvedValue(mockFeeds)
    vi.mocked(api.fetchOutbox).mockResolvedValue([])
    vi.mocked(api.fetchUsers).mockResolvedValue([])
  })

  it('verifies session and shows Login panel if unauthorized', async () => {
    vi.mocked(api.fetchStats).mockRejectedValue(new Error('Unauthorized'))

    render(App)

    // Initially shows loading verification
    expect(screen.getByText('Verifying operator session...')).toBeInTheDocument()

    // Wait for the auth failure state to render LoginPanel
    const title = await screen.findByText('rss2go aggregate')
    expect(title).toBeInTheDocument()
  })

  it('logs in automatically if active session is found', async () => {
    render(App)

    // Should auto-transition to feeds dashboard tab
    const title = await screen.findByText('rss2go panel')
    expect(title).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Feeds' })).toBeInTheDocument()
    expect(screen.getByText('Tech Blog')).toBeInTheDocument()
  })

  it('can navigate between dashboard sidebar tabs', async () => {
    render(App)

    // Find sidebar and tabs
    await screen.findByText('rss2go panel')
    
    // Switch to Subscribers tab
    const subTab = screen.getByRole('button', { name: '👤 Subscribers' })
    await fireEvent.click(subTab)
    expect(screen.getByRole('heading', { name: 'Syndication Subscribers' })).toBeInTheDocument()

    // Switch to System Stats tab
    const statsTab = screen.getByRole('button', { name: '📊 System Stats' })
    await fireEvent.click(statsTab)
    expect(screen.getByRole('heading', { name: 'System Telemetry' })).toBeInTheDocument()

    // Switch to Live Logs tab
    const logsTab = screen.getByRole('button', { name: '💻 Live Logs' })
    await fireEvent.click(logsTab)
    expect(screen.getByRole('heading', { name: 'Aggregator Console Logs' })).toBeInTheDocument()
  })

  it('triggers logout and returns to operator login prompt', async () => {
    vi.mocked(api.logout).mockResolvedValue(true)

    render(App)

    await screen.findByText('rss2go panel')
    
    const logoutBtn = screen.getByRole('button', { name: 'Logout Panel' })
    await fireEvent.click(logoutBtn)

    expect(api.logout).toHaveBeenCalled()
    const loginTitle = await screen.findByText('rss2go aggregate')
    expect(loginTitle).toBeInTheDocument()
    expect(screen.getByText('Logged out successfully')).toBeInTheDocument()
  })
})
