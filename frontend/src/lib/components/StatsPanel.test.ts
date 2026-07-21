import { render, screen, fireEvent } from '@testing-library/svelte'
import { describe, it, expect, vi } from 'vitest'
import StatsPanel from './StatsPanel.svelte'

describe('StatsPanel', () => {
  const mockOnRefresh = vi.fn()

  it('renders loading message when stats is null', () => {
    render(StatsPanel, { stats: null, outboxItems: [], onRefresh: mockOnRefresh })
    expect(screen.getByText('Fetching telemetry data...')).toBeInTheDocument()
  })

  it('renders correct telemetry values when stats is provided', () => {
    const mockStats = {
      total_feeds: 12,
      total_users: 45,
      outbox_pending: 2,
      outbox_failed: 1,
      outbox_delivered: 98,
      mailer_mode: 'smtp'
    }

    render(StatsPanel, { stats: mockStats, outboxItems: [], onRefresh: mockOnRefresh })

    expect(screen.getByText('12')).toBeInTheDocument()
    expect(screen.getByText('45')).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument()
    expect(screen.getByText('1')).toBeInTheDocument()
    expect(screen.getByText('98')).toBeInTheDocument()
    expect(screen.getByText('Mailer: smtp')).toBeInTheDocument()
    expect(screen.getByText('No recent outbox items found.')).toBeInTheDocument()
  })

  it('renders outbox transmissions correctly', () => {
    const mockStats = {
      total_feeds: 1,
      total_users: 1,
      outbox_pending: 0,
      outbox_failed: 0,
      outbox_delivered: 1,
      mailer_mode: 'smtp'
    }
    const mockOutboxItems = [
      {
        id: 101,
        recipients: ['test@example.com'],
        subject: 'Weekly Digest',
        status: 'delivered',
        retry_count: 0,
        created_at: '2026-07-07T12:00:00Z'
      }
    ]

    render(StatsPanel, { stats: mockStats, outboxItems: mockOutboxItems, onRefresh: mockOnRefresh })

    expect(screen.getByText('#101')).toBeInTheDocument()
    expect(screen.getByText('test@example.com')).toBeInTheDocument()
    expect(screen.getByText('Weekly Digest')).toBeInTheDocument()
    expect(screen.getByText('delivered')).toBeInTheDocument()
    expect(screen.getByText('0 retries')).toBeInTheDocument()
  })

  it('triggers onRefresh when clicking Refresh button', async () => {
    const mockStats = {
      total_feeds: 1,
      total_users: 1,
      outbox_pending: 0,
      outbox_failed: 0,
      outbox_delivered: 0,
      mailer_mode: 'smtp'
    }

    render(StatsPanel, { stats: mockStats, outboxItems: [], onRefresh: mockOnRefresh })

    const refreshBtn = screen.getByRole('button', { name: 'Refresh Counters' })
    await fireEvent.click(refreshBtn)

    expect(mockOnRefresh).toHaveBeenCalled()
  })
})
