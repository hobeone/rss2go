<script lang="ts">
  import { onMount } from 'svelte';

  let logLines = $state<string[]>([]);
  let isLogsConnected = $state(false);
  let eventSource = $state<EventSource | null>(null);
  let consoleContainer = $state<HTMLDivElement | null>(null);

  function connectLogs() {
    disconnectLogs();
    logLines = [];
    const es = new EventSource('/api/v1/logs');
    eventSource = es;
    isLogsConnected = true;

    es.onmessage = (event) => {
      logLines.push(event.data);
      if (logLines.length > 500) {
        logLines.shift();
      }
      setTimeout(() => {
        if (consoleContainer) {
          consoleContainer.scrollTop = consoleContainer.scrollHeight;
        }
      }, 50);
    };

    es.onerror = () => {
      isLogsConnected = false;
      es.close();
    };
  }

  function disconnectLogs() {
    if (eventSource) {
      eventSource.close();
      eventSource = null;
    }
    isLogsConnected = false;
  }

  onMount(() => {
    connectLogs();
    return () => disconnectLogs();
  });
</script>

<div style="margin-bottom: 32px;">
  <h1 class="m-title-large">Aggregator Console Logs</h1>
  <p class="m-body-medium">Live Server-Sent Events logging console directly from the polling daemon.</p>
</div>

<div class="console-pane">
  <div class="console-header">
    <div class="console-indicator">
      <span class="indicator-dot {isLogsConnected ? 'indicator-dot-active' : ''}"></span>
      <span style="font-weight: bold; font-size: 0.85rem;">
        {isLogsConnected ? 'Streaming Live Logs' : 'Stream Closed / Retry reconnecting'}
      </span>
    </div>
    <button class="m-btn m-btn-text" style="color: white; font-size: 0.8rem; padding: 4px 10px;" onclick={connectLogs}>
      Reconnect Terminal
    </button>
  </div>

  <div class="console-output" bind:this={consoleContainer}>
    {#each logLines as line, index (index)}
      {@const isError = line.includes("level=ERROR") || line.includes("level=error") || line.includes("ERR")}
      {@const isWarn = line.includes("level=WARN") || line.includes("level=warn") || line.includes("WARN") || line.includes("WRN")}
      <span class="console-line {isError ? 'console-line-error' : isWarn ? 'console-line-warn' : 'console-line-info'}">
        {line}
      </span>
    {:else}
      <span class="console-line" style="color: #838085; text-align: center; margin-top: 40px;">
        Terminal listener connected. Waiting for daemon crawl triggers or system actions...
      </span>
    {/each}
  </div>
</div>
