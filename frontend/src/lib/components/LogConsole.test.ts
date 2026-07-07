import { render, screen, fireEvent, act } from '@testing-library/svelte'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import LogConsole from './LogConsole.svelte'

class MockEventSource {
  onmessage: ((event: { data: string }) => void) | null = null
  onerror: (() => void) | null = null
  close = vi.fn()
  constructor(public url: string) {
    MockEventSource.instances.push(this)
  }
  static instances: MockEventSource[] = []
}

describe('LogConsole', () => {
  beforeEach(() => {
    MockEventSource.instances = []
    vi.stubGlobal('EventSource', MockEventSource)
  })

  it('renders waiting state initially when no logs', () => {
    render(LogConsole)
    expect(screen.getByText('Terminal listener connected. Waiting for daemon crawl triggers or system actions...')).toBeInTheDocument()
  })

  it('renders log lines streaming from EventSource', async () => {
    render(LogConsole)
    
    const instance = MockEventSource.instances[0]
    expect(instance).toBeDefined()
    expect(instance.url).toBe('/api/v1/logs')

    // Simulate sending log messages
    await act(() => {
      instance.onmessage?.({ data: 'level=INFO msg="starting crawl"' })
      instance.onmessage?.({ data: 'level=WARN msg="db slow"' })
      instance.onmessage?.({ data: 'level=ERROR msg="failed fetch"' })
    })

    expect(screen.getByText('level=INFO msg="starting crawl"')).toBeInTheDocument()
    expect(screen.getByText('level=WARN msg="db slow"')).toBeInTheDocument()
    expect(screen.getByText('level=ERROR msg="failed fetch"')).toBeInTheDocument()
  })

  it('handles disconnect and reconnect operations', async () => {
    render(LogConsole)

    const instance = MockEventSource.instances[0]
    expect(instance).toBeDefined()

    // Trigger error (disconnect)
    await act(() => {
      instance.onerror?.()
    })

    expect(screen.getByText('Stream Closed / Retry reconnecting')).toBeInTheDocument()

    // Click reconnect button
    const reconnectBtn = screen.getByRole('button', { name: 'Reconnect Terminal' })
    await fireEvent.click(reconnectBtn)

    expect(MockEventSource.instances.length).toBe(2)
    expect(screen.getByText('Streaming Live Logs')).toBeInTheDocument()
  })
})
