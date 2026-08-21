<script lang="ts">
  import { onMount, tick } from 'svelte';

  type WebMessages = {
    title: string;
    subtitle: string;
    loginHeading: string;
    loginInstructions: string;
    noPending: string;
    alreadyRegistered: string;
    sessionExpired: string;
    formHeading: string;
    authenticatedAs: string;
    username: string;
    usernameHint: string;
    email: string;
    password: string;
    confirmPassword: string;
    passwordHint: string;
    passwordMismatch: string;
    submit: string;
    submitting: string;
    success: string;
    conflict: string;
    validationFailed: string;
    upstreamFailed: string;
    internalError: string;
    retryLogin: string;
    neverSharePassword: string;
    telegramLoginFailed: string;
    loading: string;
  };

  type WebConfig = {
    language: string;
    botUsername: string;
    messages: WebMessages;
  };

  type TelegramUser = Record<string, string | number | boolean | undefined>;
  type Phase = 'loading' | 'login' | 'form' | 'success' | 'fatal';

  let config: WebConfig | null = null;
  let phase: Phase = 'loading';
  let displayName = '';
  let csrfToken = '';
  let username = '';
  let email = '';
  let password = '';
  let confirmation = '';
  let createdUsername = '';
  let errorMessage = '';
  let submitting = false;
  const usernamePattern = '[A-Za-z0-9](?:[A-Za-z0-9._-]{0,38}[A-Za-z0-9])?';

  const telegramWindow = window as typeof window & {
    onTelegramAuth?: (user: TelegramUser) => void;
  };

  onMount(() => {
    void initialize();
    return () => {
      delete telegramWindow.onTelegramAuth;
    };
  });

  async function initialize() {
    try {
      const response = await fetch('/forge/api/config', { credentials: 'same-origin' });
      if (!response.ok) throw new Error('config');
      config = await response.json() as WebConfig;
      document.documentElement.lang = config.language;
      document.title = config.messages.title;
      await restoreSession();
    } catch {
      phase = 'fatal';
    }
  }

  async function restoreSession() {
    const response = await fetch('/forge/api/session', { credentials: 'same-origin' });
    if (response.ok) {
      const session = await response.json() as { displayName: string; csrfToken: string };
      displayName = session.displayName;
      csrfToken = session.csrfToken;
      phase = 'form';
      return;
    }
    await showLogin();
  }

  async function showLogin(message = '') {
    errorMessage = message;
    phase = 'login';
    await tick();
    mountTelegramWidget();
  }

  function mountTelegramWidget() {
    if (!config) return;
    const container = document.getElementById('telegram-login');
    if (!container) return;
    container.replaceChildren();
    telegramWindow.onTelegramAuth = authenticateTelegram;
    const script = document.createElement('script');
    script.async = true;
    script.src = 'https://telegram.org/js/telegram-widget.js?22';
    script.setAttribute('data-telegram-login', config.botUsername);
    script.setAttribute('data-size', 'large');
    script.setAttribute('data-radius', '10');
    script.setAttribute('data-lang', config.language);
    script.setAttribute('data-onauth', 'onTelegramAuth(user)');
    container.appendChild(script);
  }

  async function authenticateTelegram(user: TelegramUser) {
    if (!config) return;
    errorMessage = '';
    const allowed = ['id', 'first_name', 'last_name', 'username', 'photo_url', 'auth_date', 'hash'];
    const payload: Record<string, string> = {};
    for (const key of allowed) {
      const value = user[key];
      if (value !== undefined && value !== null && value !== '') payload[key] = String(value);
    }
    try {
      const response = await fetch('/forge/api/auth/telegram', {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      const body = await response.json() as { displayName?: string; csrfToken?: string; error?: string };
      if (!response.ok || !body.displayName || !body.csrfToken) {
        errorMessage = body.error || config.messages.telegramLoginFailed;
        return;
      }
      displayName = body.displayName;
      csrfToken = body.csrfToken;
      phase = 'form';
    } catch {
      errorMessage = config.messages.telegramLoginFailed;
    }
  }

  async function submitRegistration() {
    if (!config || submitting) return;
    errorMessage = '';
    if (password !== confirmation) {
      errorMessage = config.messages.passwordMismatch;
      return;
    }
    submitting = true;
    try {
      const response = await fetch('/forge/api/register', {
        method: 'POST',
        credentials: 'same-origin',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': csrfToken
        },
        body: JSON.stringify({ username, email, password })
      });
      const body = await response.json() as { username?: string; error?: string };
      password = '';
      confirmation = '';
      if (response.ok && body.username) {
        createdUsername = body.username;
        phase = 'success';
        return;
      }
      errorMessage = body.error || config.messages.internalError;
      if (response.status === 401 || response.status === 403) {
        await showLogin(errorMessage);
      }
    } catch {
      password = '';
      confirmation = '';
      errorMessage = config.messages.internalError;
    } finally {
      submitting = false;
    }
  }
</script>

<svelte:head>
  <meta name="description" content={config?.messages.subtitle ?? 'Forgejo account registration'} />
</svelte:head>

<main>
  <section class="card" aria-live="polite">
    <div class="mark" aria-hidden="true">⌘</div>
    {#if config}
      <header>
        <p class="eyebrow">ASNK FORGE</p>
        <h1>{config.messages.title}</h1>
        <p class="subtitle">{config.messages.subtitle}</p>
      </header>
    {/if}

    {#if phase === 'loading'}
      <div class="status"><span class="spinner"></span>{config?.messages.loading ?? 'Loading…'}</div>
    {:else if phase === 'fatal'}
      <div class="notice error">Unable to load the registration service.</div>
    {:else if phase === 'login' && config}
      <div class="panel">
        <h2>{config.messages.loginHeading}</h2>
        <p>{config.messages.loginInstructions}</p>
        {#if errorMessage}<div class="notice error">{errorMessage}</div>{/if}
        <div id="telegram-login" class="telegram-login"></div>
      </div>
    {:else if phase === 'form' && config}
      <div class="panel">
        <h2>{config.messages.formHeading}</h2>
        <p class="identity"><span>{config.messages.authenticatedAs}</span> <strong>{displayName}</strong></p>
        {#if errorMessage}<div class="notice error">{errorMessage}</div>{/if}
        <form on:submit|preventDefault={submitRegistration}>
          <label>
            <span>{config.messages.username}</span>
            <input bind:value={username} name="username" autocomplete="username" minlength="1" maxlength="40" pattern={usernamePattern} required />
            <small>{config.messages.usernameHint}</small>
          </label>
          <label>
            <span>{config.messages.email}</span>
            <input bind:value={email} name="email" type="email" autocomplete="email" maxlength="254" required />
          </label>
          <label>
            <span>{config.messages.password}</span>
            <input bind:value={password} name="password" type="password" autocomplete="new-password" minlength="8" maxlength="256" required />
            <small>{config.messages.passwordHint}</small>
          </label>
          <label>
            <span>{config.messages.confirmPassword}</span>
            <input bind:value={confirmation} name="confirmation" type="password" autocomplete="new-password" minlength="8" maxlength="256" required />
          </label>
          <p class="privacy">{config.messages.neverSharePassword}</p>
          <button type="submit" disabled={submitting}>{submitting ? config.messages.submitting : config.messages.submit}</button>
        </form>
      </div>
    {:else if phase === 'success' && config}
      <div class="panel success-panel">
        <div class="success-icon" aria-hidden="true">✓</div>
        <h2>{config.messages.success}</h2>
        <p class="created-name">{createdUsername}</p>
      </div>
    {/if}
  </section>
</main>
