import { render, screen, fireEvent } from '@testing-library/svelte'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import LogConsole from './LogConsole.svelte'

const mockClose = vi.fn()
const mockMessages: Record<string, ((e: MessageEvent) => void)> = {}
const mockErrors: Record<string, ((e: Event) => void)> = {}

let instanceCount = 0
let lastUrl = ''

class MockEventSource {
  url: string
  onmessage: ((e: MessageEvent) => void) | null = null
  onerror: ((e: Event) => void) | null = null
  private id: number

  constructor(url: string) {
    this.url = url
    lastUrl = url
    this.id = ++instanceCount
    mockMessages[this.id] = (e) => this.onmessage?.(e)
    mockErrors[this.id] = (e) => this.onerror?.(e)
  }

  close = mockClose
}

function triggerMessage(id: number, data: string) {
  mockMessages[id]?.({ data } as MessageEvent)
}

function triggerError(id: number) {
  mockErrors[id]?.({} as Event)
}

describe('LogConsole', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    mockClose.mockClear()
    instanceCount = 0
    lastUrl = ''
    vi.stubGlobal('EventSource', MockEventSource)
  })

  it('connects to SSE at default INFO level and shows streaming indicator', () => {
    render(LogConsole)
    expect(lastUrl).toBe('/api/v1/logs?level=info')
    expect(screen.getByText('Streaming Live Logs')).toBeInTheDocument()
  })

  it('shows empty-state hint while no lines have arrived', () => {
    render(LogConsole)
    expect(screen.getByText(/Connecting to daemon log stream/)).toBeInTheDocument()
  })

  it('renders incoming SSE log lines in the console output', async () => {
    render(LogConsole)
    const id = instanceCount
    triggerMessage(id, '13:00:01 INFO component=scheduler msg="poll cycle started"')
    triggerMessage(id, '13:00:02 WARN component=crawler msg="retrying feed"')
    expect(await screen.findByText(/poll cycle started/)).toBeInTheDocument()
    expect(screen.getByText(/retrying feed/)).toBeInTheDocument()
  })

  it('marks connection as closed on SSE error', async () => {
    render(LogConsole)
    const id = instanceCount
    triggerError(id)
    expect(await screen.findByText(/Stream Closed/)).toBeInTheDocument()
  })

  it('displays the server-configured log level badge', () => {
    render(LogConsole, { props: { logLevel: 'debug' } })
    expect(screen.getByText('debug')).toBeInTheDocument()
  })

  it('reconnects with new level URL when level filter button is clicked', async () => {
    render(LogConsole)
    expect(lastUrl).toBe('/api/v1/logs?level=info')

    const debugBtn = screen.getByRole('button', { name: 'DEBUG' })
    await fireEvent.click(debugBtn)

    expect(mockClose).toHaveBeenCalled()
    expect(lastUrl).toBe('/api/v1/logs?level=debug')
  })

  it('reconnects when Reconnect Terminal button is clicked', async () => {
    render(LogConsole)
    const firstId = instanceCount

    const reconnectBtn = screen.getByRole('button', { name: 'Reconnect Terminal' })
    await fireEvent.click(reconnectBtn)

    expect(mockClose).toHaveBeenCalled()
    expect(instanceCount).toBeGreaterThan(firstId)
  })
})
