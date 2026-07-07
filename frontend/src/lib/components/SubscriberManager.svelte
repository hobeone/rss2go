<script lang="ts">
  import { onMount } from 'svelte';
  import * as api from '../api';

  interface Props {
    feeds: any[];
    triggerToast: (msg: string) => void;
  }

  let { feeds, triggerToast }: Props = $props();

  let users = $state<any[]>([]);
  let activeUser = $state<any>(null);
  let userSearchQuery = $state('');
  let userForm = $state({ email: '' });

  let filteredFeeds = $derived(
    feeds.filter(feed => 
      feed.title.toLowerCase().includes(userSearchQuery.toLowerCase()) ||
      feed.url.toLowerCase().includes(userSearchQuery.toLowerCase())
    )
  );

  async function loadUsers() {
    const data = await api.fetchUsers();
    if (data !== null) {
      users = data;
      if (activeUser) {
        const found = users.find(u => u.id === activeUser.id);
        activeUser = found || null;
      }
    }
  }

  async function submitUserForm(e: SubmitEvent) {
    e.preventDefault();
    if (!userForm.email.trim()) return;
    const res = await api.addUser(userForm.email);
    if (res) {
      triggerToast('User created successfully');
      userForm.email = '';
      await loadUsers();
    }
  }

  async function deleteUser(id: number) {
    if (!confirm('Delete user email address? All subscription rows will be removed.')) {
      return;
    }
    const res = await api.deleteUser(id);
    if (res !== null) {
      if (activeUser && activeUser.id === id) {
        activeUser = null;
      }
      triggerToast('User removed');
      await loadUsers();
    }
  }

  async function toggleSubscription(feedId: number, isSubscribed: boolean) {
    if (!activeUser) return;
    if (!isSubscribed) {
      await api.addSubscription(activeUser.id, feedId);
      triggerToast('Subscribed successfully');
    } else {
      await api.deleteSubscription(activeUser.id, feedId);
      triggerToast('Unsubscribed successfully');
    }
    await loadUsers();
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
      await loadUsers();
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
      await loadUsers();
    }
  }

  function openSubscriptions(user: any) {
    if (activeUser && activeUser.id === user.id) {
      activeUser = null;
      userSearchQuery = '';
    } else {
      activeUser = user;
      userSearchQuery = '';
    }
  }

  onMount(() => {
    loadUsers();
  });
</script>

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
