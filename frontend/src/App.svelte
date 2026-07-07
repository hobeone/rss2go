<script lang="ts">
  import { onMount } from 'svelte';
  import LoginPanel from './lib/components/LoginPanel.svelte';
  import StatsPanel from './lib/components/StatsPanel.svelte';
  import LogConsole from './lib/components/LogConsole.svelte';
  import * as api from './lib/api';

  // Navigation & Authentication
  let isLoggedIn = $state(false);
  let checkedLogin = $state(false);
  const savedTab = typeof window !== 'undefined' ? localStorage.getItem('rss2go_active_tab') : null;
  let currentTab = $state(
    (savedTab === 'feeds' || savedTab === 'users' || savedTab === 'stats' || savedTab === 'logs') 
      ? savedTab 
      : 'feeds'
  );

  // Primary Data
  let feeds = $state<any[]>([]);
  let users = $state<any[]>([]);
  let stats = $state<any>(null);
  let outboxItems = $state<any[]>([]);

  // Feed Actions & Modals
  let activeFeed = $state<any>(null);
  let feedSearchQuery = $state('');
  let feedFilterStatus = $state('all');
  let filteredDashboardFeeds = $derived(
    feeds.filter(feed => {
      const matchesSearch = feed.title.toLowerCase().includes(feedSearchQuery.toLowerCase()) ||
                            feed.url.toLowerCase().includes(feedSearchQuery.toLowerCase());
      const matchesFilter = feedFilterStatus === 'all' || (feed.last_error_str && feed.last_error_str !== "");
      return matchesSearch && matchesFilter;
    })
  );
  let activeFeedItems = $state<any[]>([]);
  let isLoadingFeedItems = $state(false);
  let feedItemsError = $state('');
  let isAddFeedOpen = $state(false);
  let isEditFeedOpen = $state(false);
  let isTestResultOpen = $state(false);
  let testResult = $state.raw<any>(null);
  let isTestingFeed = $state(false);
  let rewindLimit = $state(10);
  let showActionToast = $state('');

  // User Actions & Modals
  let activeUser = $state<any>(null);
  let userSearchQuery = $state('');
  let filteredFeeds = $derived(
    feeds.filter(feed => 
      feed.title.toLowerCase().includes(userSearchQuery.toLowerCase()) ||
      feed.url.toLowerCase().includes(userSearchQuery.toLowerCase())
    )
  );

  // Log Console State is now managed inside LogConsole component

  // Form structures
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

  let userForm = $state({
    email: ''
  });

  // apiFetch has been refactored to src/lib/api.ts

  // Toast Helper
  function triggerToast(msg: string) {
    showActionToast = msg;
    setTimeout(() => {
      if (showActionToast === msg) {
        showActionToast = '';
      }
    }, 4000);
  }

  // Auth Operations
  async function checkAuthStatus() {
    try {
      const data = await api.fetchStats();
      if (data) {
        isLoggedIn = true;
        stats = data;
        loadCurrentTabData();
      }
    } catch (e) {
      // Ignored: will trigger login prompt
    } finally {
      checkedLogin = true;
    }
  }

  // handleLogin has been moved to LoginPanel component

  async function handleLogout() {
    await api.logout();
    isLoggedIn = false;
    feeds = [];
    users = [];
    stats = null;
    triggerToast('Logged out successfully');
  }

  // Data Loading
  async function loadFeeds() {
    const data = await api.fetchFeeds();
    if (data !== null) {
      feeds = data;
    }
  }

  async function loadUsers() {
    const data = await api.fetchUsers();
    if (data !== null) {
      users = data;
      if (activeUser) {
        const found = users.find(u => u.id === activeUser.id);
        if (found) {
          activeUser = found;
        }
      }
    }
  }

  async function loadOutbox() {
    const data = await api.fetchOutbox();
    if (data) outboxItems = data;
  }

  async function loadStats() {
    const data = await api.fetchStats();
    if (data) stats = data;
    loadOutbox();
  }

  function loadCurrentTabData() {
    if (!isLoggedIn) return;
    if (currentTab === 'feeds') loadFeeds();
    if (currentTab === 'users') loadUsers();
    if (currentTab === 'stats') loadStats();
  }

  // SSE logging logic has been moved to LogConsole component

  // Watchers & Effects
  $effect(() => {
    loadCurrentTabData();
  });

  $effect(() => {
    localStorage.setItem('rss2go_active_tab', currentTab);
  });

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

  // Logging connection effect is now managed within LogConsole component lifecycle

  onMount(() => {
    checkAuthStatus();
  });

  // Feed Form Operations
  function openAddFeed() {
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
      extraction_strategy: feed.extraction_strategy,
      css_selector: feed.css_selector || ''
    };
    isEditFeedOpen = true;
  }

  async function submitFeedForm(e: SubmitEvent) {
    e.preventDefault();
    const payload = {
      title: feedForm.title,
      url: feedForm.url,
      poll_interval_secs: Number(feedForm.poll_interval_secs),
      backoff_factor: Number(feedForm.backoff_factor),
      extract_full_article: feedForm.extract_full_article,
      extraction_strategy: feedForm.extraction_strategy,
      css_selector: feedForm.css_selector || null
    };

    if (feedForm.id === 0) {
      // Create new
      const res = await api.addFeed(payload);
      if (res) {
        triggerToast('Feed created successfully');
        isAddFeedOpen = false;
        loadFeeds();
      }
    } else {
      // Edit existing
      const res = await api.updateFeed(feedForm.id, payload);
      if (res) {
        triggerToast('Feed updated successfully');
        isEditFeedOpen = false;
        if (activeFeed && activeFeed.id === feedForm.id) {
          activeFeed = res;
        }
        loadFeeds();
      }
    }
  }

  async function deleteFeed(id: number) {
    if (!confirm('Are you sure you want to delete this feed? All subscribers will be unsubscribed.')) {
      return;
    }
    const res = await api.deleteFeed(id);
    if (res !== null) {
      triggerToast('Feed deleted');
      activeFeed = null;
      loadFeeds();
    }
  }

  // Feed Actions
  async function testCrawl(feed: any) {
    isTestingFeed = true;
    testResult = null;
    isTestResultOpen = true;
    try {
      const data = await api.testFeed(feed);
      if (data) {
        testResult = data;
      }
    } catch (e) {
      isTestResultOpen = false;
    } finally {
      isTestingFeed = false;
    }
  }

  async function catchupFeed(id: number) {
    const res = await api.catchupFeed(id);
    if (res) {
      triggerToast(`Caught up: ${res.items_marked} items marked seen.`);
      loadFeeds();
    }
  }

  async function rewindFeed(id: number) {
    const res = await api.rewindFeed(id, { limit: Number(rewindLimit) });
    if (res) {
      triggerToast(`Rewound successfully.`);
      loadFeeds();
    }
  }

  async function scanFeedNow(id: number) {
    const res = await api.scanFeed(id);
    if (res) {
      triggerToast('Feed scan triggered');
      loadFeeds();
    }
  }

  // User Operations
  async function submitUserForm(e: SubmitEvent) {
    e.preventDefault();
    if (!userForm.email.trim()) return;
    const res = await api.addUser(userForm.email);
    if (res) {
      triggerToast('User created successfully');
      userForm.email = '';
      loadUsers();
    }
  }

  async function deleteUser(id: number) {
    if (!confirm('Delete user email address? All subscription rows will be removed.')) {
      return;
    }
    const res = await api.deleteUser(id);
    if (res !== null) {
      triggerToast('User removed');
      loadUsers();
    }
  }

  // Subscription toggle matrix helper
  async function toggleSubscription(feedId: number, isSubscribed: boolean) {
    if (!activeUser) return;
    if (!isSubscribed) {
      // Create subscription
      await api.addSubscription(activeUser.id, feedId);
      triggerToast('Subscribed successfully');
    } else {
      // Remove subscription
      await api.deleteSubscription(activeUser.id, feedId);
      triggerToast('Unsubscribed successfully');
    }
    // Reload users data to reflect totals
    loadUsers();
  }

  async function selectAllFiltered() {
    if (!activeUser) return;
    const promises = [];
    for (const feed of filteredFeeds) {
      const isSubbed = activeUser.subscribed_feed_ids?.includes(feed.id);
      if (!isSubbed) {
        promises.push(api.addSubscription(activeUser.id, feed.id));
      }
    }
    if (promises.length > 0) {
      await Promise.all(promises);
      triggerToast('Subscribed to all filtered feeds');
      loadUsers();
    }
  }

  async function deselectAllFiltered() {
    if (!activeUser) return;
    const promises = [];
    for (const feed of filteredFeeds) {
      const isSubbed = activeUser.subscribed_feed_ids?.includes(feed.id);
      if (isSubbed) {
        promises.push(api.deleteSubscription(activeUser.id, feed.id));
      }
    }
    if (promises.length > 0) {
      await Promise.all(promises);
      triggerToast('Unsubscribed from all filtered feeds');
      loadUsers();
    }
  }

  function openSubscriptions(user: any) {
    if (activeUser && activeUser.id === user.id) {
      activeUser = null;
      userSearchQuery = '';
    } else {
      activeUser = user;
      userSearchQuery = '';
      loadFeeds();
    }
  }

  function handleKeyDown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      activeFeed = null;
      activeUser = null;
      isAddFeedOpen = false;
      isEditFeedOpen = false;
      isTestResultOpen = false;
    }
  }
</script>

<svelte:window onkeydown={handleKeyDown} />

{#if !checkedLogin}
  <div class="login-container">
    <div class="m-card">
      <p class="m-title-small">Verifying operator session...</p>
    </div>
  </div>
{:else}
  {#if !isLoggedIn}
    <!-- Operator Auth Prompt -->
    <div class="login-container">
      <LoginPanel onLoginSuccess={() => { isLoggedIn = true; triggerToast('Login successful'); checkAuthStatus(); }} />
    </div>
  {:else}
    <!-- Main Dashboard layout -->
    <div class="dashboard-layout">
      <!-- Nav Sidebar -->
      <aside class="sidebar">
        <div class="sidebar-brand">
          <span class="brand-icon">⚡</span>
          <h2 class="m-title-small" style="font-weight: 600;">rss2go panel</h2>
        </div>

        <nav class="sidebar-nav">
          <button
            class="m-btn m-btn-text nav-item {currentTab === 'feeds' ? 'nav-item-active' : ''}"
            onclick={() => currentTab = 'feeds'}
          >
            <span>📰</span> Feeds
          </button>
          <button
            class="m-btn m-btn-text nav-item {currentTab === 'users' ? 'nav-item-active' : ''}"
            onclick={() => currentTab = 'users'}
          >
            <span>👤</span> Subscribers
          </button>
          <button
            class="m-btn m-btn-text nav-item {currentTab === 'stats' ? 'nav-item-active' : ''}"
            onclick={() => currentTab = 'stats'}
          >
            <span>📊</span> System Stats
          </button>
          <button
            class="m-btn m-btn-text nav-item {currentTab === 'logs' ? 'nav-item-active' : ''}"
            onclick={() => currentTab = 'logs'}
          >
            <span>💻</span> Live Logs
          </button>
        </nav>

        <div style="padding: 16px; border-top: 1px solid var(--md-sys-color-outline-variant);">
          <button class="m-btn m-btn-outlined" style="width: 100%; justify-content: center;" onclick={handleLogout}>
            Logout Panel
          </button>
        </div>
      </aside>

      <!-- Main viewport area -->
      <main class="main-content">

        {#if currentTab === 'feeds'}
          <!-- FEEDS MANAGER TAB -->
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 32px;">
            <div>
              <h1 class="m-title-large">Feeds</h1>
              <p class="m-body-medium">Add, configure, and inspect feed polling rules.</p>
            </div>
            <button class="m-btn m-btn-filled" onclick={openAddFeed}>
              <span>+</span> Add Feed Source
            </button>
          </div>

          <!-- Search / Filter input -->
          <div class="m-card" style="margin-bottom: 32px; padding: 16px;">
            <div style="display: flex; flex-wrap: wrap; gap: 16px; align-items: flex-end;">
              <div class="m-input-group" style="margin: 0; flex: 1; min-width: 250px; max-width: 500px;">
                <span class="m-input-label">Filter Feeds list</span>
                <input
                  type="text"
                  placeholder="Search feed title or URL..."
                  class="m-input"
                  bind:value={feedSearchQuery}
                />
              </div>
              <div style="display: flex; align-items: center; gap: 8px; height: 48px;">
                <button
                  type="button"
                  class="m-btn {feedFilterStatus === 'all' ? 'm-btn-filled' : 'm-btn-outlined'}"
                  onclick={() => feedFilterStatus = 'all'}
                >
                  All Feeds
                </button>
                <button
                  type="button"
                  class="m-btn {feedFilterStatus === 'error' ? 'm-btn-filled' : 'm-btn-outlined'}"
                  style={feedFilterStatus === 'error' ? 'background-color: var(--md-sys-color-error); color: var(--md-sys-color-on-error); border-color: var(--md-sys-color-error);' : ''}
                  onclick={() => feedFilterStatus = 'error'}
                >
                  ⚠️ Errors Only
                </button>
              </div>
            </div>
          </div>

          <div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: 24px;">
            {#each filteredDashboardFeeds as feed (feed.id)}
              <div
                class="m-card m-card-interactive"
                role="button"
                tabindex="0"
                onclick={() => activeFeed = feed}
                onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { activeFeed = feed; e.preventDefault(); } }}
              >
                <div style="display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 12px;">
                  <h3 class="m-title-small" style="font-weight: 600; text-overflow: ellipsis; overflow: hidden; white-space: nowrap; max-width: 220px;">
                    {feed.title || 'Untitled Feed'}
                  </h3>
                  <span class="m-badge {feed.last_error_str ? 'm-badge-error' : 'm-badge-primary'}">
                    {feed.last_error_str ? 'Error' : 'Active'}
                  </span>
                </div>
                <p class="m-body-medium" style="word-break: break-all; margin-bottom: 16px; font-size: 0.85rem;">
                  {feed.url}
                </p>
                <div style="display: flex; gap: 16px; border-top: 1px solid var(--md-sys-color-outline-variant); padding-top: 12px; font-size: 0.85rem; color: var(--md-sys-color-on-surface-variant);">
                  <div>⏱️ {feed.poll_interval_secs}s</div>
                  <div>🔄 x{feed.backoff_factor}</div>
                  {#if feed.extract_full_article}
                    <div>📄 Extractor</div>
                  {/if}
                </div>
              </div>
            {:else}
              <div class="m-card" style="grid-column: 1 / -1; text-align: center; padding: 48px; color: var(--md-sys-color-on-surface-variant);">
                <span style="font-size: 2.5rem; display: block; margin-bottom: 12px;">🔍</span>
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
                      {activeFeed.url} 🔗
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
                    <h4 style="color: var(--md-sys-color-on-error-container); font-weight: bold; margin-bottom: 4px;">Crawling Error Alert</h4>
                    <p style="color: var(--md-sys-color-on-error-container); font-size: 0.85rem; word-break: break-all;">
                      {activeFeed.last_error_str}
                    </p>
                  </div>
                  {/if}
                </div>

                <!-- Feed Items List -->
                <div style="border-top: 1px solid var(--md-sys-color-outline-variant); padding-top: 16px; display: flex; flex-direction: column; gap: 8px;">
                  <h4 class="m-title-small" style="font-size: 0.95rem; font-weight: bold; display: flex; align-items: center; gap: 8px;">
                    📰 Recent Feed Items ({activeFeedItems.length})
                  </h4>

                  {#if isLoadingFeedItems}
                    <div style="text-align: center; padding: 16px; color: var(--md-sys-color-on-surface-variant); font-size: 0.85rem;">
                      Fetching feed items...
                    </div>
                  {:else if feedItemsError}
                    <div style="color: var(--md-sys-color-error); font-size: 0.85rem; padding: 8px; border: 1px solid var(--md-sys-color-error); border-radius: var(--radius-sm); background-color: var(--md-sys-color-error-container);">
                      ⚠ {feedItemsError}
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
                          <span class="m-badge {item.seen ? 'm-badge-primary' : 'm-badge-secondary'}" style="font-size: 0.7rem; padding: 2px 8px; flex-shrink: 0;">
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
                  <h4 class="m-title-small" style="font-size: 0.95rem; font-weight: bold;">Operator Control Operations</h4>
                  <div style="display: flex; flex-wrap: wrap; gap: 12px;">
                    <button class="m-btn m-btn-tonal" onclick={() => testCrawl(activeFeed)}>
                      🧪 Test Feed Dry-run
                    </button>
                    <button class="m-btn class-btn-outlined m-btn-outlined" onclick={() => catchupFeed(activeFeed.id)}>
                      ✔️ Catch Up (Mark Seen)
                    </button>
                    <button class="m-btn m-btn-outlined" onclick={() => scanFeedNow(activeFeed.id)}>
                      ⚡ Run Scan Now
                    </button>
                    <div style="display: flex; align-items: center; gap: 8px;">
                      <button class="m-btn m-btn-outlined" onclick={() => rewindFeed(activeFeed.id)}>
                        ⏪ Rewind
                      </button>
                      <input type="number" class="m-input" style="width: 70px; padding: 8px 12px;" bind:value={rewindLimit} min="1" max="500" />
                      <span class="m-body-medium">items</span>
                    </div>
                  </div>
                </div>

                <div style="margin-top: 16px; display: flex; justify-content: space-between; border-top: 1px solid var(--md-sys-color-outline-variant); padding-top: 16px;">
                  <button class="m-btn m-btn-tonal" onclick={() => { openEditFeed(activeFeed); }}>
                    ✏️ Edit Feed Config
                  </button>
                  <button class="m-btn m-btn-error" onclick={() => deleteFeed(activeFeed.id)}>
                    🗑️ Delete
                  </button>
                </div>
              </div>
            </div>
          {/if}

        {:else}
          <!-- Other views -->
        {/if}

        {#if currentTab === 'users'}
          <!-- SUBSCRIBERS / USERS TAB -->
          <div style="margin-bottom: 32px;">
            <h1 class="m-title-large">Syndication Subscribers</h1>
            <p class="m-body-medium">Register subscriber email addresses and tie them to crawled feeds.</p>
          </div>

          <div class="subscribers-split-layout">
            <!-- Left Pane: Master Management -->
            <div class="subscribers-master-pane">
              <!-- Add User Card Form -->
              <div class="m-card" style="padding: 20px;">
                <h3 class="m-title-small" style="margin-bottom: 12px; font-weight: 600;">Register New Recipient Email</h3>
                <form onsubmit={submitUserForm} style="display: flex; gap: 12px; align-items: flex-end;">
                  <div class="m-input-group" style="flex-grow: 1;">
                    <span class="m-input-label">Subscriber Email Address</span>
                    <input type="email" placeholder="subscriber@example.com" class="m-input" bind:value={userForm.email} required />
                  </div>
                  <button type="submit" class="m-btn m-btn-filled" style="height: 48px;">
                    Add Subscriber
                  </button>
                </form>
              </div>

              <!-- Users List Table -->
              <div class="m-card" style="padding: 0; overflow: hidden;">
                <table class="feed-matrix-table">
                  <thead>
                    <tr>
                      <th>Subscriber Email</th>
                      <th style="width: 80px;">ID</th>
                      <th style="text-align: right;">Removal</th>
                    </tr>
                  </thead>
                  <tbody>
                    {#each users as user (user.id)}
                      {@const isSelected = activeUser && activeUser.id === user.id}
                      <tr 
                        class="subscriber-row {isSelected ? 'active-subscriber-row' : ''}"
                        style="cursor: pointer;"
                        onclick={() => openSubscriptions(user)}
                      >
                        <td style="font-weight: 500;">
                          {user.email}
                          {#if isSelected}
                            <span style="margin-left: 8px; font-size: 0.8rem; opacity: 0.85;">(selected)</span>
                          {/if}
                        </td>
                        <td>{user.id}</td>
                        <td style="text-align: right;">
                          <button 
                            class="m-btn m-btn-text" 
                            style="color: var(--md-sys-color-error); padding: 4px;" 
                            onclick={(e) => { e.stopPropagation(); deleteUser(user.id); }}
                          >
                            Remove
                          </button>
                        </td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              </div>
            </div>

            <!-- Right Pane: Detail Subscription Matrix -->
            <div class="subscribers-detail-pane m-card">
              {#if activeUser}
                <div style="display: flex; justify-content: space-between; align-items: flex-start; border-bottom: 1px solid var(--md-sys-color-outline-variant); padding-bottom: 12px;">
                  <div>
                    <h3 class="m-title-small" style="font-weight: 600;">Subscriptions Matrix</h3>
                    <p class="m-body-medium" style="font-size: 0.85rem; color: var(--md-sys-color-on-surface-variant); word-break: break-all; margin-top: 4px;">
                      Managing feeds for: <strong>{activeUser.email}</strong>
                    </p>
                  </div>
                  <button class="m-btn m-btn-text" style="padding: 4px;" onclick={() => { activeUser = null; userSearchQuery = ''; }}>✕</button>
                </div>

                <!-- Search Input -->
                <div class="m-input-group">
                  <span class="m-input-label">Filter Feeds list</span>
                  <input
                    type="text"
                    placeholder="Search feed title or URL..."
                    class="m-input"
                    bind:value={userSearchQuery}
                  />
                </div>

                <!-- Bulk Selection Controls -->
                <div class="bulk-select-controls">
                  <button class="m-btn m-btn-tonal" style="flex: 1;" onclick={selectAllFiltered}>
                    ✔️ Select All Filtered ({filteredFeeds.length})
                  </button>
                  <button class="m-btn m-btn-outlined" style="flex: 1;" onclick={deselectAllFiltered}>
                    ❌ Deselect All Filtered
                  </button>
                </div>

                <!-- Scrollable Checklist -->
                <div class="feeds-checklist-scroll">
                  {#each filteredFeeds as feed (feed.id)}
                    {@const isSubbed = activeUser.subscribed_feed_ids?.includes(feed.id)}
                    <!-- svelte-ignore a11y_label_has_associated_control -->
                    <label class="checklist-item" style="margin: 0; display: flex; width: 100%;">
                      <input
                        type="checkbox"
                        class="m-checkbox"
                        checked={isSubbed}
                        onclick={(e) => {
                          e.stopPropagation();
                          toggleSubscription(feed.id, isSubbed);
                        }}
                      />
                      <div style="display: flex; flex-direction: column; gap: 2px;">
                        <span style="font-weight: 500; font-size: 0.9rem;">{feed.title || 'Untitled Feed'}</span>
                        <span style="font-size: 0.75rem; color: var(--md-sys-color-on-surface-variant); word-break: break-all;">{feed.url}</span>
                      </div>
                    </label>
                  {:else}
                    <div style="text-align: center; padding: 24px; color: var(--md-sys-color-on-surface-variant); font-size: 0.9rem;">
                      No feeds match your search criteria.
                    </div>
                  {/each}
                </div>
              {:else}
                <div style="display: flex; flex-direction: column; align-items: center; justify-content: center; height: 100%; min-height: 250px; text-align: center; color: var(--md-sys-color-on-surface-variant); gap: 12px;">
                  <span style="font-size: 3rem; opacity: 0.75;">📬</span>
                  <p class="m-body-medium" style="max-width: 250px;">Select a subscriber from the list to audit and configure their feed subscriptions.</p>
                </div>
              {/if}
            </div>
          </div>

        {:else}
          <!-- Other views -->
        {/if}

        {#if currentTab === 'stats'}
          <StatsPanel {stats} {outboxItems} onRefresh={loadStats} />
        {/if}

        {#if currentTab === 'logs'}
          <LogConsole />
        {/if}

      </main>
    </div>
  {/if}
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
                <option value="css">Targeted Custom CSS Selector</option>
              </select>
            </div>
            {#if feedForm.extraction_strategy === 'css'}
              <div class="m-input-group">
                <span class="m-input-label">Custom DOM CSS Selector (e.g. article.post-content)</span>
                <input type="text" placeholder="article.post-body" class="m-input" bind:value={feedForm.css_selector} required />
              </div>
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
          <h2 class="m-title-medium">🧪 Test Crawl Dry-Run Report</h2>
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
                    <h5 class="m-input-label" style="margin-bottom: 6px; font-weight: bold; color: var(--md-sys-color-primary);">📄 Full-text Extractor Preview (First Item Only)</h5>
                    <div style="background-color: var(--md-sys-color-surface); padding: 12px; border-radius: var(--radius-sm); font-size: 0.85rem; max-height: 180px; overflow-y: auto; text-align: left; line-height: 1.6; border: 1px solid var(--md-sys-color-outline-variant);">
                      {@html item.extracted_content}
                    </div>
                  </div>
                {/if}

                <div style="border-top: 1px dashed var(--md-sys-color-outline); padding-top: 8px; margin-top: 4px;">
                  <h5 class="m-input-label" style="margin-bottom: 6px; font-weight: bold;">🧹 HTML Sanitizer & Relative Paths Resolution Output</h5>
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

<!-- Toast Alerts Notification Bar -->
{#if showActionToast}
  <div style="position: fixed; bottom: 24px; right: 24px; z-index: 1000; animation: slideIn 0.3s cubic-bezier(0.2, 0, 0, 1);" class="m-card">
    <div style="display: flex; align-items: center; gap: 12px; padding: 4px 8px;">
      <span style="font-size: 1.2rem;">🔔</span>
      <span style="font-weight: 500; font-size: 0.95rem;">{showActionToast}</span>
    </div>
  </div>
{/if}

<style>
  @keyframes slideIn {
    from { transform: translateY(40px); opacity: 0; }
    to { transform: translateY(0); opacity: 1; }
  }
</style>
