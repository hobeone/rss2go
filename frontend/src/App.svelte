<script lang="ts">
  import { onMount } from 'svelte';
  import StatsPanel from './lib/components/StatsPanel.svelte';
  import LogConsole from './lib/components/LogConsole.svelte';
  import SubscriberManager from './lib/components/SubscriberManager.svelte';
  import FeedManager from './lib/components/FeedManager.svelte';
  import * as api from './lib/api';

  // Navigation
  const savedTab = typeof window !== 'undefined' ? localStorage.getItem('rss2go_active_tab') : null;
  let currentTab = $state(
    (savedTab === 'feeds' || savedTab === 'users' || savedTab === 'stats' || savedTab === 'logs') 
      ? savedTab 
      : 'feeds'
  );

  // Primary Data
  let feeds = $state<any[]>([]);
  let stats = $state<any>(null);
  let outboxItems = $state<any[]>([]);

  // Feed Actions & Modals state
  let showActionToast = $state('');

  // Toast Helper
  function triggerToast(msg: string) {
    showActionToast = msg;
    setTimeout(() => {
      if (showActionToast === msg) {
        showActionToast = '';
      }
    }, 4000);
  }

  // Data Loading
  async function loadFeeds() {
    try {
      const data = await api.fetchFeeds();
      if (data !== null) {
        feeds = data;
      }
    } catch (e) {
      console.error(e);
    }
  }

  async function loadOutbox() {
    try {
      const data = await api.fetchOutbox();
      if (data) outboxItems = data;
    } catch (e) {
      console.error(e);
    }
  }

  async function loadStats() {
    try {
      const data = await api.fetchStats();
      if (data) stats = data;
      loadOutbox();
    } catch (e) {
      console.error(e);
    }
  }

  function loadCurrentTabData() {
    if (currentTab === 'feeds') loadFeeds();
    if (currentTab === 'stats') loadStats();
  }

  // Watchers & Effects
  $effect(() => {
    loadCurrentTabData();
  });

  $effect(() => {
    localStorage.setItem('rss2go_active_tab', currentTab);
  });

  onMount(() => {
    loadCurrentTabData();
  });

  function handleKeyDown(e: KeyboardEvent) {
    // Esc can be handled locally if needed
  }
</script>

<svelte:window onkeydown={handleKeyDown} />

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
  </aside>

  <!-- Main viewport area -->
  <main class="main-content">

    {#if currentTab === 'feeds'}
      <FeedManager {feeds} {triggerToast} onRefresh={loadFeeds} />
    {/if}

    {#if currentTab === 'users'}
      <SubscriberManager {feeds} {triggerToast} />
    {/if}

    {#if currentTab === 'stats'}
      <StatsPanel {stats} {outboxItems} onRefresh={loadStats} />
    {/if}

    {#if currentTab === 'logs'}
      <LogConsole logLevel={stats?.log_level ?? 'info'} />
    {/if}

  </main>
</div>

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
