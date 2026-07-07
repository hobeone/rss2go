import { render, screen, fireEvent, act } from '@testing-library/svelte'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import SubscriberManager from './SubscriberManager.svelte'
import * as api from '../api'

vi.mock('../api', () => ({
  fetchUsers: vi.fn(),
  addUser: vi.fn(),
  deleteUser: vi.fn(),
  addSubscription: vi.fn(),
  deleteSubscription: vi.fn()
}))

describe('SubscriberManager', () => {
  const mockFeeds = [
    { id: 1, title: 'Tech Blog', url: 'https://tech.com/feed' },
    { id: 2, title: 'Cooking Blog', url: 'https://cook.com/feed' }
  ]
  const mockUsers = [
    { id: 101, email: 'alice@example.com', subscribed_feed_ids: [1] },
    { id: 102, email: 'bob@example.com', subscribed_feed_ids: [] }
  ]
  const mockTriggerToast = vi.fn()

  beforeEach(() => {
    vi.restoreAllMocks()
    mockTriggerToast.mockClear()
    vi.mocked(api.fetchUsers).mockResolvedValue(mockUsers)
  })

  it('renders correctly and fetches users on mount', async () => {
    render(SubscriberManager, { feeds: mockFeeds, triggerToast: mockTriggerToast })

    expect(screen.getByText('Syndication Subscribers')).toBeInTheDocument()
    
    // Wait for the async list render
    const aliceRow = await screen.findByText('alice@example.com')
    expect(aliceRow).toBeInTheDocument()
    expect(screen.getByText('bob@example.com')).toBeInTheDocument()
    expect(screen.getByText('Select a subscriber from the list to audit and configure their feed subscriptions.')).toBeInTheDocument()
  })

  it('opens details pane on subscriber row click', async () => {
    render(SubscriberManager, { feeds: mockFeeds, triggerToast: mockTriggerToast })

    const aliceRow = await screen.findByText('alice@example.com')
    await fireEvent.click(aliceRow)

    expect(screen.getByText('Subscriptions Matrix')).toBeInTheDocument()
    expect(screen.getByText('Managing feeds for:')).toBeInTheDocument()
    expect(screen.getAllByText('alice@example.com').length).toBe(2)

    // Checklist items
    expect(screen.getByText('Tech Blog')).toBeInTheDocument()
    expect(screen.getByText('Cooking Blog')).toBeInTheDocument()

    const checkboxes = screen.getAllByRole('checkbox')
    expect(checkboxes[0]).toBeChecked() // Tech Blog is subbed
    expect(checkboxes[1]).not.toBeChecked() // Cooking Blog is not subbed
  })

  it('registers new subscriber email on form submit', async () => {
    vi.mocked(api.addUser).mockResolvedValue({ id: 103, email: 'carol@example.com' })

    render(SubscriberManager, { feeds: mockFeeds, triggerToast: mockTriggerToast })

    const emailInput = screen.getByPlaceholderText('subscriber@example.com')
    const addBtn = screen.getByRole('button', { name: 'Add Subscriber' })

    await fireEvent.input(emailInput, { target: { value: 'carol@example.com' } })
    await fireEvent.click(addBtn)

    expect(api.addUser).toHaveBeenCalledWith('carol@example.com')
    expect(mockTriggerToast).toHaveBeenCalledWith('User created successfully')
  })

  it('removes recipient email on delete click', async () => {
    vi.mocked(api.deleteUser).mockResolvedValue(true)
    vi.stubGlobal('confirm', () => true)

    render(SubscriberManager, { feeds: mockFeeds, triggerToast: mockTriggerToast })

    await screen.findByText('alice@example.com')
    const removeButtons = screen.getAllByRole('button', { name: 'Remove' })
    await fireEvent.click(removeButtons[0]) // click Alice's remove button

    expect(api.deleteUser).toHaveBeenCalledWith(101)
    expect(mockTriggerToast).toHaveBeenCalledWith('User removed')
  })
})
