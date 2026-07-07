<script lang="ts">
  import { onMount } from 'svelte';

  const LEVELS = ['DEBUG', 'INFO', 'WARN', 'ERROR'] as const;
  type Level = typeof LEVELS[number];

  interface Props {
    logLevel?: string; // server-configured level, e.g. "info"
  }

  let { logLevel = 'info' }: Props = $props();

  let logLines = $state<string[]>([]);
  let isLogsConnected = $state(false);
  let eventSource = $state<EventSource | null>(null);
  let consoleContainer = $state<HTMLDivElement | null>(null);
  let selectedLevel = $state<Level>('INFO');

  function lineLevel(line: string): number {
    const up = line.toUpperCase();
    if (up.includes('LEVEL=ERROR') || up.includes('ERR') || up.includes('LEVEL=ERROR')) return 3;
    if (up.includes('LEVEL=WARN') || up.includes('WRN')) return 2;
    if (up.includes('LEVEL=INFO') || up.includes('INF')) return 1;
    if (up.includes('LEVEL=DEBUG') || up.includes('DBG')) return 0;
    return -1; // unknown — always show
  }

  function connectLogs() {
    disconnectLogs();
    logLines = [];
    const url = `/api/v1/logs?level=${selectedLevel.toLowerCase()}`;
    const es = new EventSource(url);
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

  function changeLevel(lvl: Level) {
    selectedLevel = lvl;
    connectLogs();
  }

  onMount(() => {
    connectLogs();
    return () => disconnectLogs();
  });
</script>

<div style="margin-bottom: 32px; display: flex; justify-content: space-between; align-items: flex-end; flex-wrap: wrap; gap: 16px;">
  <div>
    <h1 class="m-title-large">Aggregator Console Logs</h1>
    <p class="m-body-medium">Live structured log stream from the polling daemon — scheduling, crawls, DB activity, and errors.</p>
  </div>
  <div style="display: flex; align-items: center; gap: 12px;">
    <div style="display: flex; align-items: center; gap: 6px;">
      <span class="m-body-medium" style="font-size: 0.8rem; color: var(--md-sys-color-on-surface-variant);">Server level:</span>
      <span class="m-badge m-badge-primary" style="font-size: 0.75rem; text-transform: uppercase; letter-spacing: 0.05em;">
        {logLevel}
      </span>
    </div>
    <div style="display: flex; gap: 4px;" role="group" aria-label="Log level filter">
      {#each LEVELS as lvl (lvl)}
        <button
          type="button"
          class="m-btn {selectedLevel === lvl ? 'm-btn-filled' : 'm-btn-outlined'}"
          style="padding: 4px 12px; font-size: 0.75rem;"
          onclick={() => changeLevel(lvl)}
          aria-pressed={selectedLevel === lvl}
        >
          {lvl}
        </button>
      {/each}
    </div>
  </div>
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
      {@const isError = line.toUpperCase().includes('LEVEL=ERROR') || line.includes('ERR')}
      {@const isWarn = line.toUpperCase().includes('LEVEL=WARN') || line.includes('WRN')}
      <span class="console-line {isError ? 'console-line-error' : isWarn ? 'console-line-warn' : 'console-line-info'}">
        {line}
      </span>
    {:else}
      <span class="console-line" style="color: #838085; text-align: center; margin-top: 40px;">
        Connecting to daemon log stream... Set level to DEBUG to see scheduler and DB activity.
      </span>
    {/each}
  </div>
</div>
