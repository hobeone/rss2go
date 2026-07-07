<script lang="ts">
  import { login } from '../api';

  interface Props {
    onLoginSuccess: () => void;
  }

  let { onLoginSuccess }: Props = $props();

  let passwordInput = $state('');
  let loginError = $state('');

  async function handleLogin(e: SubmitEvent) {
    e.preventDefault();
    loginError = '';
    try {
      const ok = await login(passwordInput);
      if (ok) {
        passwordInput = '';
        onLoginSuccess();
      } else {
        loginError = 'Invalid credentials';
      }
    } catch (err: any) {
      loginError = err.message || 'Network error';
    }
  }
</script>

<div class="m-card login-card">
  <h2 class="m-title-medium" style="margin-bottom: 8px;">rss2go aggregate</h2>
  <p class="m-body-medium" style="margin-bottom: 24px;">Please authenticate to access the operator panel.</p>

  <form onsubmit={handleLogin} style="display: flex; flex-direction: column; gap: 20px;">
    <div class="m-input-group">
      <span class="m-input-label">Operator Password</span>
      <input
        type="password"
        placeholder="••••••••"
        class="m-input"
        bind:value={passwordInput}
        required
        autocomplete="current-password"
      />
    </div>

    {#if loginError}
      <p class="m-body-medium" style="color: var(--md-sys-color-error); font-weight: 500;">
        {loginError}
      </p>
    {/if}

    <button type="submit" class="m-btn m-btn-filled" style="width: 100%;">
      Unlock Panel
    </button>
  </form>
</div>
