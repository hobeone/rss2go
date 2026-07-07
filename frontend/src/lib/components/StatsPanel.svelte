<script lang="ts">
  interface Props {
    stats: any;
    outboxItems: any[];
    onRefresh: () => void;
  }

  let { stats, outboxItems, onRefresh }: Props = $props();
</script>

<div style="margin-bottom: 32px;">
  <h1 class="m-title-large">System Telemetry</h1>
  <p class="m-body-medium">Real-time outbox queues and subscriber totals.</p>
</div>

{#if stats}
  <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 24px; margin-bottom: 32px;">
    <div class="m-card" style="text-align: center;">
      <h3 class="m-input-label" style="margin-bottom: 8px;">Total Feeds</h3>
      <span class="m-title-large" style="color: var(--md-sys-color-primary);">{stats.total_feeds}</span>
    </div>
    <div class="m-card" style="text-align: center;">
      <h3 class="m-input-label" style="margin-bottom: 8px;">Total Subscribers</h3>
      <span class="m-title-large" style="color: var(--md-sys-color-primary);">{stats.total_users}</span>
    </div>
    <div class="m-card" style="text-align: center;">
      <h3 class="m-input-label" style="margin-bottom: 8px;">Outbox Pending</h3>
      <span class="m-title-large" style="color: #FFB300;">{stats.outbox_pending}</span>
    </div>
    <div class="m-card" style="text-align: center;">
      <h3 class="m-input-label" style="margin-bottom: 8px;">Outbox Failed</h3>
      <span class="m-title-large" style="color: var(--md-sys-color-error);">{stats.outbox_failed}</span>
    </div>
    <div class="m-card" style="text-align: center;">
      <h3 class="m-input-label" style="margin-bottom: 8px;">Outbox Delivered</h3>
      <span class="m-title-large" style="color: #4CAF50;">{stats.outbox_delivered}</span>
    </div>
  </div>

  <div class="m-card" style="margin-bottom: 32px; padding: 24px;">
    <h3 class="m-title-small" style="margin-bottom: 16px; font-weight: 600; display: flex; align-items: center; justify-content: space-between; gap: 8px;">
      <span>📬 Recent Outbox Transmissions (Email Audit Log)</span>
      {#if stats.mailer_mode}
        <span class="m-badge m-badge-secondary" style="font-size: 0.8rem; text-transform: uppercase;">
          Mailer: {stats.mailer_mode}
        </span>
      {/if}
    </h3>
    
    <div style="overflow-x: auto; border: 1px solid var(--md-sys-color-outline-variant); border-radius: var(--radius-md);">
      <table class="feed-matrix-table" style="margin-bottom: 0;">
        <thead>
          <tr>
            <th style="width: 80px;">ID</th>
            <th>Recipients</th>
            <th>Subject</th>
            <th>Status</th>
            <th style="width: 140px;">Attempts</th>
            <th style="width: 180px;">Date Generated</th>
          </tr>
        </thead>
        <tbody>
          {#each outboxItems as item (item.id)}
            <tr>
              <td style="font-weight: 500; font-size: 0.85rem;">#{item.id}</td>
              <td style="font-size: 0.85rem; max-width: 200px; word-break: break-all;">
                {item.recipients ? item.recipients.join(', ') : ''}
              </td>
              <td style="font-size: 0.85rem; font-weight: 500;">{item.subject}</td>
              <td>
                <span class="m-badge {item.status === 'delivered' ? 'm-badge-primary' : item.status === 'pending' ? 'm-badge-secondary' : 'm-badge-error'}">
                  {item.status}
                </span>
              </td>
              <td style="font-size: 0.85rem;">
                {item.retry_count} attempts
                {#if item.last_error}
                  <div style="color: var(--md-sys-color-error); font-size: 0.75rem; margin-top: 4px; max-width: 250px; word-break: break-all;" title={item.last_error}>
                    ⚠ {item.last_error}
                  </div>
                {/if}
              </td>
              <td style="font-size: 0.85rem; color: var(--md-sys-color-on-surface-variant);">
                {new Date(item.created_at).toLocaleString()}
              </td>
            </tr>
          {:else}
            <tr>
              <td colspan="6" style="text-align: center; color: var(--md-sys-color-on-surface-variant); padding: 24px;">
                No recent outbox items found.
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </div>
{:else}
  <p class="m-body-medium">Fetching telemetry data...</p>
{/if}

<button class="m-btn m-btn-tonal" onclick={onRefresh}>
  🔄 Refresh Counters
</button>
