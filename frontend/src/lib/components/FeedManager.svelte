<script lang="ts">
  import { onMount } from 'svelte';
  import * as api from '../api';

  interface Props {
    feeds: any[];
    triggerToast: (msg: string) => void;
    onRefresh: () => Promise<void>;
  }

  let { feeds, triggerToast, onRefresh }: Props = $props();

  let activeFeed = $state<any>(null);
  let feedSearchQuery = $state('');
  let feedFilterStatus = $state('all');
  let activeFeedItems = $state<any[]>([]);
  let isLoadingFeedItems = $state(false);
  let feedItemsError = $state('');
  let isAddFeedOpen = $state(false);
  let isEditFeedOpen = $state(false);
  let isTestResultOpen = $state(false);
  let testResult = $state.raw<any>(null);
  let isTestingFeed = $state(false);
  let rewindLimit = $state(10);
  let pendingDeleteId = $state<number | null>(null);
  let nowTick = $state(Date.now());

  let users = $state<any[]>([]);
  let subscribeAll = $state(false);
  let selectedUserIDs = $state<number[]>([]);

  let feedForm = $state({
    id: 0,
    title: '',
    url: '',
    poll_interval_secs: 600,
    backoff_factor: 1.5,
    extract_full_article: false,
    extraction_strategy: 'heuristic',
    css_selector: ''
  });

  let filteredDashboardFeeds = $derived(
    feeds.filter(feed => {
      const matchesSearch = feed.title.toLowerCase().includes(feedSearchQuery.toLowerCase()) ||
                            feed.url.toLowerCase().includes(feedSearchQuery.toLowerCase());
      const matchesFilter = feedFilterStatus === 'all' || (feed.last_error_str && feed.last_error_str !== "");
      return matchesSearch && matchesFilter;
    })
  );

  async function loadFeedItems(feedId: number) {
    isLoadingFeedItems = true;
    feedItemsError = '';
    activeFeedItems = [];
    try {
      const data = await api.fetchFeedItems(feedId);
      if (data) {
        activeFeedItems = data;
      }
    } catch (e: any) {
      feedItemsError = e.message || 'Failed to fetch items';
    } finally {
      isLoadingFeedItems = false;
    }
  }

  $effect(() => {
    if (activeFeed) {
      loadFeedItems(activeFeed.id);
    } else {
      activeFeedItems = [];
      feedItemsError = '';
    }
  });

  async function openAddFeed() {
    feedForm = {
      id: 0,
      title: '',
      url: '',
      poll_interval_secs: 600,
      backoff_factor: 1.5,
      extract_full_article: false,
      extraction_strategy: 'heuristic',
      css_selector: ''
    };
    subscribeAll = false;
    selectedUserIDs = [];
    users = [];
    try {
      const data = await api.fetchUsers();
      if (data) {
        users = data;
      }
    } catch (e) {
      console.error(e);
    }
    isAddFeedOpen = true;
  }

  function openEditFeed(feed: any) {
    feedForm = {
      id: feed.id,
      title: feed.title,
      url: feed.url,
      poll_interval_secs: feed.poll_interval_secs,
      backoff_factor: feed.backoff_factor,
      extract_full_article: feed.extract_full_article,
      extraction_strategy: feed.extraction_strategy || 'heuristic',
      css_selector: feed.css_selector || ''
    };
    isEditFeedOpen = true;
  }

  async function submitFeedForm(e: SubmitEvent) {
    e.preventDefault();
    const payload: any = {
      title: feedForm.title,
      url: feedForm.url,
      poll_interval_secs: Number(feedForm.poll_interval_secs),
      backoff_factor: Number(feedForm.backoff_factor),
      extract_full_article: feedForm.extract_full_article,
      extraction_strategy: feedForm.extraction_strategy,
      css_selector: feedForm.css_selector || null
    };
    if (isAddFeedOpen) {
      payload.subscribe_all = subscribeAll;
      payload.subscribe_user_ids = selectedUserIDs;
    }
    try {
      if (isAddFeedOpen) {
        const res = await api.addFeed(payload);
        if (res) {
          triggerToast('Feed created successfully');
          isAddFeedOpen = false;
          await onRefresh();
        }
      } else {
        const res = await api.updateFeed(feedForm.id, payload);
        if (res) {
          triggerToast('Feed updated successfully');
          isEditFeedOpen = false;
          if (activeFeed && activeFeed.id === feedForm.id) {
            activeFeed = { ...activeFeed, ...payload };
          }
          await onRefresh();
        }
      }
    } catch (err: any) {
      triggerToast(err.message || 'Failed to submit form');
    }
  }

  async function confirmDeleteFeed(id: number) {
    pendingDeleteId = null;
    try {
      const res = await api.deleteFeed(id);
      if (res !== null) {
        triggerToast('Feed deleted');
        activeFeed = null;
        await onRefresh();
      }
    } catch (err: any) {
      triggerToast(err.message || 'Failed to delete feed');
    }
  }

  async function testCrawl(feed: any) {
    isTestingFeed = true;
    testResult = null;
    isTestResultOpen = true;
    try {
      const data = await api.testFeed(feed);
      testResult = data;
    } catch (err: any) {
      triggerToast('Testing dry-run crawl failed: ' + err.message);
      isTestResultOpen = false;
    } finally {
      isTestingFeed = false;
    }
  }

  async function catchupFeed(id: number) {
    try {
      const res = await api.catchupFeed(id);
      if (res) {
        triggerToast(`Caught up: ${res.items_marked} items marked seen.`);
        if (activeFeed && activeFeed.id === id) {
          await loadFeedItems(id);
        }
      }
    } catch (err: any) {
      triggerToast(err.message || 'Failed to catch up feed');
    }
  }

  async function scanFeedNow(id: number) {
    try {
      const res = await api.scanFeed(id);
      if (res) {
        triggerToast('Feed scan triggered');
        if (activeFeed && activeFeed.id === id) {
          await loadFeedItems(id);
        }
      }
    } catch (err: any) {
      triggerToast(err.message || 'Failed to trigger scan');
    }
  }

  onMount(() => {
    const timer = setInterval(() => { nowTick = Date.now(); }, 1000);
    return () => clearInterval(timer);
  });

  // Signature element: fraction of the poll interval elapsed since the last crawl,
  // driving the amber pulse-bar fill on each feed card.
  function pollCycleProgress(feed: any): { pct: number; label: string } {
    const intervalMs = (feed.poll_interval_secs || 1) * 1000;
    const next = feed.next_poll_at ? new Date(feed.next_poll_at).getTime() : null;
    if (!next) return { pct: 0, label: 'awaiting first poll' };
    const remainingMs = next - nowTick;
    const pct = Math.min(100, Math.max(0, 100 - (remainingMs / intervalMs) * 100));
    if (remainingMs <= 0) return { pct: 100, label: 'polling due' };
    const remainingSecs = Math.round(remainingMs / 1000);
    const label = remainingSecs < 60
      ? `next poll in ${remainingSecs}s`
      : `next poll in ${Math.round(remainingSecs / 60)}m`;
    return { pct, label };
  }

  async function rewindFeed(id: number) {
    try {
      const res = await api.rewindFeed(id, rewindLimit);
      if (res) {
        triggerToast('Rewound successfully.');
        if (activeFeed && activeFeed.id === id) {
          await loadFeedItems(id);
        }
      }
    } catch (err: any) {
      triggerToast(err.message || 'Failed to rewind feed');
    }
  }
</script>

<!-- FEEDS MANAGER TAB -->
<div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 32px;">
  <div>
    <h1 class="m-title-large">Feeds</h1>
    <p class="m-body-medium">Add, configure, and inspect feed polling rules.</p>
  </div>
  <button class="m-btn m-btn-filled" onclick={openAddFeed}>
    Add Feed Source
  </button>
</div>

<!-- Search / Filter toolbar -->
<div class="m-toolbar">
  <div class="m-input-group" style="margin: 0; flex: 1; min-width: 250px; max-width: 500px;">
    <span class="m-input-label">Filter feeds</span>
    <input
      type="text"
      placeholder="Search feed title or URL..."
      class="m-input"
      bind:value={feedSearchQuery}
    />
  </div>
  <div style="display: flex; align-items: center; gap: 8px; height: 44px;">
    <button
      type="button"
      class="m-btn {feedFilterStatus === 'all' ? 'm-btn-filled' : 'm-btn-outlined'}"
      onclick={() => feedFilterStatus = 'all'}
    >
      All Feeds
    </button>
    <button
      type="button"
      class="m-btn {feedFilterStatus === 'error' ? 'm-btn-error' : 'm-btn-outlined'}"
      onclick={() => feedFilterStatus = 'error'}
    >
      Errors Only
    </button>
  </div>
</div>

<div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: 24px;">
  {#each filteredDashboardFeeds as feed (feed.id)}
    {@const cycle = pollCycleProgress(feed)}
    <div
      class="m-card m-card-interactive"
      role="button"
      tabindex="0"
      onclick={() => activeFeed = feed}
      onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { activeFeed = feed; e.preventDefault(); } }}
    >
      <div style="display: flex; justify-content: space-between; align-items: flex-start; gap: 12px; margin-bottom: 12px;">
        <h3 class="m-title-small" style="text-overflow: ellipsis; overflow: hidden; white-space: nowrap; min-width: 0;">
          {feed.title || 'Untitled Feed'}
        </h3>
        <span class="m-status {feed.last_error_str ? 'm-status-error' : 'm-status-ok'}" style="flex-shrink: 0;">
          {feed.last_error_str ? 'Error' : 'Active'}
        </span>
      </div>
      <p class="m-body-medium" style="word-break: break-all; margin-bottom: 16px; font-size: 0.85rem;">
        {feed.url}
      </p>
      <div style="display: flex; gap: 16px; margin-bottom: 14px; font-size: 0.8rem; font-family: var(--font-mono); color: var(--md-sys-color-on-surface-variant);">
        <div>poll {feed.poll_interval_secs}s</div>
        <div>backoff x{feed.backoff_factor}</div>
        {#if feed.extract_full_article}
          <div>extractor on</div>
        {/if}
      </div>
      <div class="poll-cycle" style="border-top: 1px solid var(--md-sys-color-outline-variant); padding-top: 12px;">
        <div class="poll-cycle-track">
          <div class="poll-cycle-fill" style="width: {cycle.pct}%;"></div>
        </div>
        <div class="poll-cycle-label">
          <span>{feed.last_error_str ? 'stalled on error' : cycle.label}</span>
        </div>
      </div>
    </div>
  {:else}
    <div class="m-card" style="grid-column: 1 / -1; text-align: center; padding: 48px; color: var(--md-sys-color-on-surface-variant);">
      <p class="m-body-medium">No feeds match your search criteria.</p>
    </div>
  {/each}
</div>

<!-- Feed Detail Panel Overlay -->
{#if activeFeed}
  <div class="m-modal-container">
    <button class="m-dialog-overlay" onclick={() => activeFeed = null} aria-label="Close panel"></button>
    <div class="m-dialog">
      <div style="display: flex; justify-content: space-between; align-items: flex-start;">
        <div>
          <h2 class="m-title-medium">{activeFeed.title}</h2>
          <a href={activeFeed.url} target="_blank" class="m-body-medium" style="color: var(--md-sys-color-primary); text-decoration: none; word-break: break-all; font-size: 0.85rem;">
            {activeFeed.url}
          </a>
        </div>
        <button class="m-btn m-btn-text" style="padding: 4px;" onclick={() => activeFeed = null}>✕</button>
      </div>

      <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 16px; border-top: 1px solid var(--md-sys-color-outline-variant); padding-top: 16px;">
        <div class="m-input-group">
          <span class="m-input-label">Last Polled</span>
          <span class="m-input" style="background-color: var(--md-sys-color-surface-variant);">
            {activeFeed.last_polled_at ? new Date(activeFeed.last_polled_at).toLocaleString() : 'Never'}
          </span>
        </div>
        <div class="m-input-group">
          <span class="m-input-label">Next Scheduled Poll</span>
          <span class="m-input" style="background-color: var(--md-sys-color-surface-variant);">
            {new Date(activeFeed.next_poll_at).toLocaleString()}
          </span>
        </div>
        <div class="m-input-group">
          <span class="m-input-label">Poll Interval</span>
          <span class="m-input" style="background-color: var(--md-sys-color-surface-variant);">
            {activeFeed.poll_interval_secs} seconds
          </span>
        </div>
        <div class="m-input-group">
          <span class="m-input-label">Backoff Factor</span>
          <span class="m-input" style="background-color: var(--md-sys-color-surface-variant);">
            {activeFeed.backoff_factor}
          </span>
        </div>
        {#if activeFeed.last_error_str}
        <div class="m-card" style="background-color: var(--md-sys-color-error-container); border-color: var(--md-sys-color-error); padding: 12px 16px; grid-column: span 2;">
          <h4 style="color: var(--md-sys-color-on-error-container); margin-bottom: 4px; font-size: 0.85rem;">Crawling Error Alert</h4>
          <p style="color: var(--md-sys-color-on-error-container); font-size: 0.85rem; word-break: break-all;">
            {activeFeed.last_error_str}
          </p>
        </div>
        {/if}
      </div>

      <!-- Feed Items List -->
      <div style="border-top: 1px solid var(--md-sys-color-outline-variant); padding-top: 16px; display: flex; flex-direction: column; gap: 8px;">
        <h4 class="m-title-small" style="font-size: 0.9rem;">
          Recent Feed Items ({activeFeedItems.length})
        </h4>

        {#if isLoadingFeedItems}
          <div style="text-align: center; padding: 16px; color: var(--md-sys-color-on-surface-variant); font-size: 0.85rem;">
            Fetching feed items...
          </div>
        {:else if feedItemsError}
          <div style="color: var(--md-sys-color-error); font-size: 0.85rem; padding: 8px; border: 1px solid var(--md-sys-color-error); border-radius: var(--radius-sm); background-color: var(--md-sys-color-error-container);">
            {feedItemsError}
          </div>
        {:else}
          <div style="max-height: 220px; overflow-y: auto; border: 1px solid var(--md-sys-color-outline-variant); border-radius: var(--radius-md); padding: 8px; background-color: var(--md-sys-color-surface); display: flex; flex-direction: column; gap: 8px;">
            {#each activeFeedItems as item}
              <div style="display: flex; justify-content: space-between; align-items: center; gap: 12px; padding: 6px 8px; border-bottom: 1px dashed var(--md-sys-color-outline-variant); font-size: 0.85rem;">
                <div style="display: flex; flex-direction: column; gap: 2px; overflow: hidden; text-align: left;">
                  <a href={item.link} target="_blank" style="font-weight: 500; color: var(--md-sys-color-primary); text-decoration: none; text-overflow: ellipsis; overflow: hidden; white-space: nowrap;" title={item.title}>
                    {item.title || 'Untitled'}
                  </a>
                  <span style="font-size: 0.75rem; color: var(--md-sys-color-on-surface-variant);">
                    {item.published_at ? new Date(item.published_at).toLocaleString() : 'No date'}
                  </span>
                </div>
                <span class="m-status {item.seen ? 'm-status-ok' : 'm-status-pending'}" style="font-size: 0.65rem; flex-shrink: 0;">
                  {item.seen ? 'Emailed' : 'Unseen'}
                </span>
              </div>
            {:else}
              <div style="text-align: center; padding: 16px; color: var(--md-sys-color-on-surface-variant); font-size: 0.85rem;">
                No items found in this feed.
              </div>
            {/each}
          </div>
        {/if}
      </div>

      <div style="border-top: 1px solid var(--md-sys-color-outline-variant); padding-top: 16px; display: flex; flex-direction: column; gap: 16px;">
        <h4 class="m-title-small" style="font-size: 0.9rem;">Operator Control Operations</h4>
        <div style="display: flex; flex-wrap: wrap; gap: 12px;">
          <button class="m-btn m-btn-tonal" onclick={() => testCrawl(activeFeed)}>
            Test Feed
          </button>
          <button class="m-btn m-btn-outlined" onclick={() => catchupFeed(activeFeed.id)}>
            Catch Up
          </button>
          <button class="m-btn m-btn-outlined" onclick={() => scanFeedNow(activeFeed.id)}>
            Scan Now
          </button>
          <div style="display: flex; align-items: center; gap: 8px;">
            <button class="m-btn m-btn-outlined" onclick={() => rewindFeed(activeFeed.id)}>
              Rewind
            </button>
            <input type="number" class="m-input" style="width: 70px; padding: 8px 12px;" bind:value={rewindLimit} min="1" max="500" />
            <span class="m-body-medium">items</span>
          </div>
        </div>
      </div>

      <div style="margin-top: 16px; display: flex; justify-content: space-between; border-top: 1px solid var(--md-sys-color-outline-variant); padding-top: 16px;">
        <button class="m-btn m-btn-tonal" onclick={() => { openEditFeed(activeFeed); }}>
          Edit Feed Config
        </button>
        <button class="m-btn m-btn-error" onclick={() => { pendingDeleteId = activeFeed.id; }}>
          Delete
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- Add Feed Config Overlay Dialog -->
{#if isAddFeedOpen || isEditFeedOpen}
  <div class="m-modal-container">
    <button class="m-dialog-overlay" onclick={() => { isAddFeedOpen = false; isEditFeedOpen = false; }} aria-label="Close form"></button>
    <div class="m-dialog">
      <h2 class="m-title-medium">{isAddFeedOpen ? 'Configure New Feed Source' : 'Edit Feed Configuration'}</h2>

      <form onsubmit={submitFeedForm} style="display: flex; flex-direction: column; gap: 16px;">
        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 16px;">
          <div class="m-input-group">
            <span class="m-input-label">Feed Title Label</span>
            <input type="text" placeholder="Engineering Blog" class="m-input" bind:value={feedForm.title} required />
          </div>
          <div class="m-input-group">
            <span class="m-input-label">Feed XML URL Address</span>
            <input type="url" placeholder="https://site.com/feed.xml" class="m-input" bind:value={feedForm.url} required />
          </div>
          <div class="m-input-group">
            <span class="m-input-label">Scheduled Polling Interval (seconds)</span>
            <input type="number" class="m-input" bind:value={feedForm.poll_interval_secs} min="30" max="86400" required />
          </div>
          <div class="m-input-group">
            <span class="m-input-label">Backoff Factor (error scaling multiplier)</span>
            <input type="number" class="m-input" bind:value={feedForm.backoff_factor} min="1.0" max="10.0" step="0.1" required />
          </div>
        </div>

        <div style="border-top: 1px solid var(--md-sys-color-outline-variant); padding-top: 16px;">
          <label class="m-checkbox-label">
            <input type="checkbox" class="m-checkbox" bind:checked={feedForm.extract_full_article} />
            Enable Full-Text Article Extraction Heuristics (Fetch Item Body Content)
          </label>
        </div>

        {#if feedForm.extract_full_article}
          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 16px; border-left: 3px solid var(--md-sys-color-primary); padding-left: 12px;" class="m-card">
            <div class="m-input-group">
              <span class="m-input-label">Extraction Strategy Selection</span>
              <select class="m-input m-select" bind:value={feedForm.extraction_strategy}>
                <option value="heuristic">Fallback Standard Readability Heuristics</option>
                <option value="selector">Targeted Custom CSS Selector</option>
              </select>
            </div>
            {#if feedForm.extraction_strategy === 'selector' || feedForm.extraction_strategy === 'css'}
              <div class="m-input-group">
                <span class="m-input-label">Custom DOM CSS Selector (e.g. article.post-content)</span>
                <input type="text" placeholder="article.post-body" class="m-input" bind:value={feedForm.css_selector} required />
              </div>
            {/if}
          </div>
        {/if}

        {#if isAddFeedOpen}
          <div style="border-top: 1px solid var(--md-sys-color-outline-variant); padding-top: 16px;">
            <span class="m-input-label" style="margin-bottom: 8px; display: block; font-weight: 500;">Feed Subscriber Allocation</span>
            <label class="m-checkbox-label" style="margin-bottom: 12px; display: flex; align-items: center; gap: 8px;">
              <input type="checkbox" class="m-checkbox" bind:checked={subscribeAll} />
              Subscribe all current users
            </label>
            
            {#if !subscribeAll && users.length > 0}
              <div style="border: 1px solid var(--md-sys-color-outline-variant); border-radius: 8px; padding: 12px; max-height: 150px; overflow-y: auto; display: flex; flex-direction: column; gap: 8px; margin-top: 8px; background: rgba(0,0,0,0.1);" class="subscriber-select-list">
                {#each users as user (user.id)}
                  <label class="m-checkbox-label" style="font-size: 0.85rem; display: flex; align-items: center; gap: 8px;">
                    <input 
                      type="checkbox" 
                      class="m-checkbox" 
                      checked={selectedUserIDs.includes(user.id)} 
                      onchange={(e) => {
                        if (e.currentTarget.checked) {
                          selectedUserIDs = [...selectedUserIDs, user.id];
                        } else {
                          selectedUserIDs = selectedUserIDs.filter(id => id !== user.id);
                        }
                      }} 
                    />
                    {user.email}
                  </label>
                {/each}
              </div>
            {:else if !subscribeAll && users.length === 0}
              <p class="m-body-medium" style="color: var(--md-sys-color-on-surface-variant); font-style: italic;">No users registered yet. Add users in the Subscribers tab.</p>
            {/if}
          </div>
        {/if}

        <div style="display: flex; justify-content: flex-end; gap: 12px; margin-top: 8px;">
          <button type="button" class="m-btn m-btn-outlined" onclick={() => { isAddFeedOpen = false; isEditFeedOpen = false; }}>
            Cancel
          </button>
          <button type="submit" class="m-btn m-btn-filled">
            {isAddFeedOpen ? 'Register Feed' : 'Save Config Changes'}
          </button>
        </div>
      </form>
    </div>
  </div>
{/if}

<!-- Crawl Test Results Dialog -->
{#if isTestResultOpen}
  <div class="m-modal-container">
    <button class="m-dialog-overlay" onclick={() => isTestResultOpen = false} aria-label="Close report"></button>
    <div class="m-dialog" style="max-width: 800px;">
      <div style="display: flex; justify-content: space-between; align-items: flex-start; border-bottom: 1px solid var(--md-sys-color-outline-variant); padding-bottom: 12px;">
        <div>
          <h2 class="m-title-medium">Test Crawl Dry-Run Report</h2>
          {#if testResult}
            <p class="m-body-medium">Parsed feed: <strong>{testResult.title || 'Unknown Title'}</strong></p>
          {/if}
        </div>
        <button class="m-btn m-btn-text" style="padding: 4px;" onclick={() => isTestResultOpen = false}>✕</button>
      </div>

      <div style="max-height: 500px; overflow-y: auto; display: flex; flex-direction: column; gap: 16px;">
        {#if isTestingFeed}
          <div style="text-align: center; padding: 40px 0;">
            <p class="m-title-small">Crawling target feeds xml URL, resolving redirects, and generating heuristics extraction previews...</p>
          </div>
        {:else if testResult}
          {#if testResult.not_modified}
            <div class="m-card" style="background-color: var(--md-sys-color-secondary-container); padding: 20px; text-align: center;">
              <h3 style="font-weight: 600; color: var(--md-sys-color-on-secondary-container);">Feed Up-To-Date (304 Not Modified)</h3>
              <p class="m-body-medium" style="margin-top: 8px;">The remote server returned HTTP Status 304. Cache validation headers (ETag / Last-Modified) matched correctly.</p>
            </div>
          {:else if !testResult.items || testResult.items.length === 0}
            <p class="m-body-medium" style="text-align: center; padding: 20px;">The feed was parsed successfully but contained 0 active items.</p>
          {:else}
            {#each testResult.items as item, index}
              <div class="m-card" style="display: flex; flex-direction: column; gap: 12px; background-color: var(--md-sys-color-surface-variant);">
                <div style="display: flex; justify-content: space-between; align-items: flex-start;">
                  <h4 style="font-weight: 600; font-size: 0.95rem; max-width: 500px;">
                    {index + 1}. {item.title || 'No Title'}
                  </h4>
                  <span class="m-badge m-badge-secondary" style="font-size: 0.75rem;">Parsed Item</span>
                </div>
                <a href={item.link} target="_blank" style="font-size: 0.8rem; color: var(--md-sys-color-primary); text-decoration: none; word-break: break-all; margin-top: -6px;">
                  {item.link}
                </a>

                {#if item.extracted_content}
                  <div style="border-top: 1px dashed var(--md-sys-color-outline); padding-top: 8px; margin-top: 8px;">
                    <h5 class="m-input-label" style="margin-bottom: 6px; color: var(--md-sys-color-primary);">Full-text Extractor Preview (First Item Only)</h5>
                    <div style="background-color: var(--md-sys-color-surface); padding: 12px; border-radius: var(--radius-sm); font-size: 0.85rem; max-height: 180px; overflow-y: auto; text-align: left; line-height: 1.6; border: 1px solid var(--md-sys-color-outline-variant);">
                      {@html item.extracted_content}
                    </div>
                  </div>
                {/if}

                <div style="border-top: 1px dashed var(--md-sys-color-outline); padding-top: 8px; margin-top: 4px;">
                  <h5 class="m-input-label" style="margin-bottom: 6px;">HTML Sanitizer & Relative Paths Resolution Output</h5>
                  <div style="background-color: var(--md-sys-color-surface); padding: 12px; border-radius: var(--radius-sm); font-size: 0.85rem; max-height: 150px; overflow-y: auto; text-align: left; line-height: 1.6; border: 1px solid var(--md-sys-color-outline-variant);">
                    {@html item.content || '<em>No body content or description tags found</em>'}
                  </div>
                </div>
              </div>
            {/each}
          {/if}
        {/if}
      </div>

      <div style="display: flex; justify-content: flex-end; border-top: 1px solid var(--md-sys-color-outline-variant); padding-top: 12px;">
        <button class="m-btn m-btn-filled" onclick={() => isTestResultOpen = false}>
          Close Report
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- Delete Confirmation Dialog -->
{#if pendingDeleteId !== null}
  <div class="m-modal-container">
    <button class="m-dialog-overlay" onclick={() => pendingDeleteId = null} aria-label="Cancel delete"></button>
    <div class="m-dialog m-dialog-danger">
      <h2 class="m-title-medium">Delete this feed?</h2>
      <p class="m-body-medium">All subscribers will be unsubscribed and its crawl history discarded. This cannot be undone.</p>
      <div style="display: flex; justify-content: flex-end; gap: 12px;">
        <button class="m-btn m-btn-outlined" onclick={() => pendingDeleteId = null}>
          Cancel
        </button>
        <button class="m-btn m-btn-error" onclick={() => confirmDeleteFeed(pendingDeleteId!)}>
          Delete Feed
        </button>
      </div>
    </div>
  </div>
{/if}
