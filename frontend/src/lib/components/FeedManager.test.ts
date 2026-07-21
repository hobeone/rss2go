import { render, screen, fireEvent } from '@testing-library/svelte'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import FeedManager from './FeedManager.svelte'
import * as api from '../api'

vi.mock('../api', () => ({
  fetchFeedItems: vi.fn(),
  addFeed: vi.fn(),
  updateFeed: vi.fn(),
  deleteFeed: vi.fn(),
  testFeed: vi.fn(),
  catchupFeed: vi.fn(),
  scanFeed: vi.fn(),
  rewindFeed: vi.fn(),
  fetchUsers: vi.fn()
}))

describe('FeedManager', () => {
  const mockFeeds = [
    { id: 1, title: 'Tech Blog', url: 'https://tech.com/feed', poll_interval_secs: 600, backoff_factor: 1.5, extract_full_article: true },
    { id: 2, title: 'Cooking Blog', url: 'https://cook.com/feed', poll_interval_secs: 1200, backoff_factor: 2.0, extract_full_article: false }
  ]
  const mockItems = [
    { id: 10, title: 'Go Release 1.24', link: 'https://tech.com/go124', published_at: '2026-07-07T12:00:00Z', seen: true }
  ]
  const mockUsers = [
    { id: 101, email: 'alice@example.com' },
    { id: 102, email: 'bob@example.com' }
  ]
  const mockTriggerToast = vi.fn()
  const mockOnRefresh = vi.fn()

  beforeEach(() => {
    vi.restoreAllMocks()
    mockTriggerToast.mockClear()
    mockOnRefresh.mockClear()
    vi.mocked(api.fetchFeedItems).mockResolvedValue(mockItems)
    vi.mocked(api.fetchUsers).mockResolvedValue(mockUsers)
  })

  it('renders feeds lists correctly', () => {
    render(FeedManager, { feeds: mockFeeds, triggerToast: mockTriggerToast, onRefresh: mockOnRefresh })

    expect(screen.getByText('Feeds')).toBeInTheDocument()
    expect(screen.getByText('Tech Blog')).toBeInTheDocument()
    expect(screen.getByText('Cooking Blog')).toBeInTheDocument()
  })

  it('opens add feed modal and submits form with default subscriber settings', async () => {
    vi.mocked(api.addFeed).mockResolvedValue({ id: 3, title: 'News' })

    render(FeedManager, { feeds: mockFeeds, triggerToast: mockTriggerToast, onRefresh: mockOnRefresh })

    const addBtn = screen.getByRole('button', { name: 'Add Feed Source' })
    await fireEvent.click(addBtn)

    expect(screen.getByText('Configure New Feed Source')).toBeInTheDocument()

    const titleInput = screen.getByPlaceholderText('Engineering Blog')
    const urlInput = screen.getByPlaceholderText('https://site.com/feed.xml')

    await fireEvent.input(titleInput, { target: { value: 'News Blog' } })
    await fireEvent.input(urlInput, { target: { value: 'https://news.com/feed.xml' } })
    
    const form = titleInput.closest('form')!
    await fireEvent.submit(form)

    expect(api.addFeed).toHaveBeenCalledWith(expect.objectContaining({
      title: 'News Blog',
      url: 'https://news.com/feed.xml',
      subscribe_all: false,
      subscribe_user_ids: []
    }))
    expect(mockTriggerToast).toHaveBeenCalledWith('Feed created successfully')
    expect(mockOnRefresh).toHaveBeenCalled()
  })

  it('opens add feed modal and submits form with subscribe_all: true', async () => {
    vi.mocked(api.addFeed).mockResolvedValue({ id: 3, title: 'News' })

    render(FeedManager, { feeds: mockFeeds, triggerToast: mockTriggerToast, onRefresh: mockOnRefresh })

    const addBtn = screen.getByRole('button', { name: 'Add Feed Source' })
    await fireEvent.click(addBtn)

    const titleInput = screen.getByPlaceholderText('Engineering Blog')
    const urlInput = screen.getByPlaceholderText('https://site.com/feed.xml')

    await fireEvent.input(titleInput, { target: { value: 'All News' } })
    await fireEvent.input(urlInput, { target: { value: 'https://all.com/feed.xml' } })

    const subscribeAllCheckbox = screen.getByLabelText('Subscribe all current users')
    await fireEvent.click(subscribeAllCheckbox)
    
    const form = titleInput.closest('form')!
    await fireEvent.submit(form)

    expect(api.addFeed).toHaveBeenCalledWith(expect.objectContaining({
      title: 'All News',
      url: 'https://all.com/feed.xml',
      subscribe_all: true,
      subscribe_user_ids: []
    }))
  })

  it('opens add feed modal and submits form with selected users', async () => {
    vi.mocked(api.addFeed).mockResolvedValue({ id: 3, title: 'News' })

    render(FeedManager, { feeds: mockFeeds, triggerToast: mockTriggerToast, onRefresh: mockOnRefresh })

    const addBtn = screen.getByRole('button', { name: 'Add Feed Source' })
    await fireEvent.click(addBtn)

    const titleInput = screen.getByPlaceholderText('Engineering Blog')
    const urlInput = screen.getByPlaceholderText('https://site.com/feed.xml')

    await fireEvent.input(titleInput, { target: { value: 'Selected News' } })
    await fireEvent.input(urlInput, { target: { value: 'https://sel.com/feed.xml' } })

    // Wait for users to load and find the checkbox for alice@example.com
    const aliceCheckbox = await screen.findByLabelText('alice@example.com')
    await fireEvent.click(aliceCheckbox)

    const form = titleInput.closest('form')!
    await fireEvent.submit(form)

    expect(api.addFeed).toHaveBeenCalledWith(expect.objectContaining({
      title: 'Selected News',
      url: 'https://sel.com/feed.xml',
      subscribe_all: false,
      subscribe_user_ids: [101]
    }))
  })

  it('opens feed details on card click', async () => {
    render(FeedManager, { feeds: mockFeeds, triggerToast: mockTriggerToast, onRefresh: mockOnRefresh })

    const card = screen.getByText('Tech Blog')
    await fireEvent.click(card)

    // Verify it fetches items and displays them
    expect(api.fetchFeedItems).toHaveBeenCalledWith(1)
    const itemLink = await screen.findByText('Go Release 1.24')
    expect(itemLink).toBeInTheDocument()

    // Control buttons should be visible
    expect(screen.getByRole('button', { name: 'Test Feed' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Scan Now' })).toBeInTheDocument()
  })
})
