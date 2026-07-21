import { render, screen, fireEvent } from '@testing-library/svelte'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import App from './App.svelte'
import * as api from './lib/api'

vi.mock('./lib/api', () => ({
  fetchStats: vi.fn(),
  fetchFeeds: vi.fn(),
  fetchOutbox: vi.fn(),
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

  it('loads feeds dashboard automatically on mount', async () => {
    render(App)

    const title = await screen.findByText('rss2go')
    expect(title).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Feeds' })).toBeInTheDocument()
    expect(screen.getByText('Tech Blog')).toBeInTheDocument()
  })

  it('can navigate between dashboard sidebar tabs', async () => {
    render(App)

    await screen.findByText('rss2go')

    // Switch to Subscribers tab
    const subTab = screen.getByRole('button', { name: 'Subscribers' })
    await fireEvent.click(subTab)
    expect(screen.getByRole('heading', { name: 'Syndication Subscribers' })).toBeInTheDocument()

    // Switch to System Stats tab
    const statsTab = screen.getByRole('button', { name: 'System Stats' })
    await fireEvent.click(statsTab)
    expect(screen.getByRole('heading', { name: 'System Telemetry' })).toBeInTheDocument()

    // Switch to Live Logs tab
    const logsTab = screen.getByRole('button', { name: 'Live Logs' })
    await fireEvent.click(logsTab)
    expect(screen.getByRole('heading', { name: 'Aggregator Console Logs' })).toBeInTheDocument()
  })
})
