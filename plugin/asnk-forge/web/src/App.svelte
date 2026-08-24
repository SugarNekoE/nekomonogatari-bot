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
  type ThemePreference = 'auto' | 'light' | 'dark';

  const themeCopyByLanguage = {
    en: {
      group: 'Page appearance',
      auto: 'Auto',
      light: 'Light',
      dark: 'Dark',
      current: 'Current appearance',
      source: 'Source code'
    },
    zh: {
      group: '页面配色',
      auto: '自动',
      light: '浅色',
      dark: '深色',
      current: '当前配色',
      source: '源代码'
    }
  } as const;

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
  let themePreference: ThemePreference = 'auto';
  let themeCopy: typeof themeCopyByLanguage.en | typeof themeCopyByLanguage.zh = themeCopyByLanguage.en;
  const usernamePattern = '[A-Za-z0-9](?:[A-Za-z0-9._-]{0,38}[A-Za-z0-9])?';

  $: themeCopy = config?.language.toLowerCase().startsWith('zh')
    ? themeCopyByLanguage.zh
    : themeCopyByLanguage.en;

  const telegramWindow = window as typeof window & {
    onTelegramAuth?: (user: TelegramUser) => void;
  };

  onMount(() => {
    const initialTheme = document.documentElement.dataset.themePreference;
    if (initialTheme === 'auto' || initialTheme === 'light' || initialTheme === 'dark') {
      themePreference = initialTheme;
    }
    const colorScheme = window.matchMedia('(prefers-color-scheme: dark)');
    const handleColorSchemeChange = () => {
      if (themePreference === 'auto') applyTheme();
    };
    colorScheme.addEventListener('change', handleColorSchemeChange);
    void initialize();
    return () => {
      colorScheme.removeEventListener('change', handleColorSchemeChange);
      delete telegramWindow.onTelegramAuth;
    };
  });

  function applyTheme() {
    const resolved = themePreference === 'auto'
      ? window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
      : themePreference;
    document.documentElement.dataset.theme = resolved;
    document.documentElement.dataset.themePreference = themePreference;
    document.documentElement.style.colorScheme = resolved;
  }

  function setTheme(preference: ThemePreference) {
    themePreference = preference;
    try {
      localStorage.setItem('asnk-forge-theme', preference);
    } catch {}
    applyTheme();
  }

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
    script.setAttribute('data-radius', '4');
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

<main class="registration-page">
  <div class="container registration-content">
    <div class="row">
      <div class="col-xs-12 col-sm-8 col-sm-offset-2 col-md-6 col-md-offset-3">
        <header class="page-toolbar clearfix">
          <a class="forge-brand pull-left" href="https://forge.asnk.io/" target="_blank" rel="noreferrer">
            <img src="/forge/forgejo.svg" alt="" width="28" height="28" />
            <span>Asnk Forge</span>
          </a>
          <div class="btn-group btn-group-sm pull-right" role="group" aria-label={themeCopy.group}>
            <button
              type="button"
              class="btn btn-default"
              class:active={themePreference === 'auto'}
              aria-pressed={themePreference === 'auto'}
              on:click={() => setTheme('auto')}
            >
              {themeCopy.auto}
            </button>
            <button
              type="button"
              class="btn btn-default"
              class:active={themePreference === 'light'}
              aria-pressed={themePreference === 'light'}
              on:click={() => setTheme('light')}
            >
              {themeCopy.light}
            </button>
            <button
              type="button"
              class="btn btn-default"
              class:active={themePreference === 'dark'}
              aria-pressed={themePreference === 'dark'}
              on:click={() => setTheme('dark')}
            >
              {themeCopy.dark}
            </button>
            <span class="sr-only" aria-live="polite">{themeCopy.current}: {themeCopy[themePreference]}</span>
          </div>
        </header>

        <section class="panel panel-default registration-panel" aria-live="polite">
          {#if config}
            <div class="panel-heading">
              <h1 class="panel-title">{config.messages.title}</h1>
            </div>
          {/if}
          <div class="panel-body">
            {#if phase === 'loading'}
              <p class="loading-state text-muted">
                <span class="glyphicon glyphicon-refresh spinning" aria-hidden="true"></span>
                {config?.messages.loading ?? 'Loading…'}
              </p>
            {:else if phase === 'fatal'}
              <div class="alert alert-danger" role="alert">Unable to load the registration service.</div>
            {:else if phase === 'login' && config}
              <h2 class="section-title">{config.messages.loginHeading}</h2>
              <p class="text-muted">{config.messages.loginInstructions}</p>
              {#if errorMessage}<div class="alert alert-danger" role="alert">{errorMessage}</div>{/if}
              <div id="telegram-login" class="telegram-login"></div>
            {:else if phase === 'form' && config}
              <h2 class="section-title">{config.messages.formHeading}</h2>
              <div class="alert alert-info identity">
                {config.messages.authenticatedAs} <strong>{displayName}</strong>
              </div>
              {#if errorMessage}<div class="alert alert-danger" role="alert">{errorMessage}</div>{/if}
              <form on:submit|preventDefault={submitRegistration}>
                <div class="form-group">
                  <label class="control-label" for="forgejo-username">{config.messages.username}</label>
                  <input
                    id="forgejo-username"
                    class="form-control"
                    bind:value={username}
                    name="username"
                    autocomplete="username"
                    minlength="1"
                    maxlength="40"
                    pattern={usernamePattern}
                    aria-describedby="forgejo-username-help"
                    required
                  />
                  <span id="forgejo-username-help" class="help-block">{config.messages.usernameHint}</span>
                </div>
                <div class="form-group">
                  <label class="control-label" for="forgejo-email">{config.messages.email}</label>
                  <input
                    id="forgejo-email"
                    class="form-control"
                    bind:value={email}
                    name="email"
                    type="email"
                    autocomplete="email"
                    maxlength="254"
                    required
                  />
                </div>
                <div class="form-group">
                  <label class="control-label" for="forgejo-password">{config.messages.password}</label>
                  <input
                    id="forgejo-password"
                    class="form-control"
                    bind:value={password}
                    name="password"
                    type="password"
                    autocomplete="new-password"
                    minlength="8"
                    maxlength="256"
                    aria-describedby="forgejo-password-help"
                    required
                  />
                  <span id="forgejo-password-help" class="help-block">{config.messages.passwordHint}</span>
                </div>
                <div class="form-group">
                  <label class="control-label" for="forgejo-password-confirmation">{config.messages.confirmPassword}</label>
                  <input
                    id="forgejo-password-confirmation"
                    class="form-control"
                    bind:value={confirmation}
                    name="confirmation"
                    type="password"
                    autocomplete="new-password"
                    minlength="8"
                    maxlength="256"
                    required
                  />
                </div>
                <p class="help-block password-notice">{config.messages.neverSharePassword}</p>
                <button class="btn btn-primary" type="submit" disabled={submitting}>
                  {submitting ? config.messages.submitting : config.messages.submit}
                </button>
              </form>
            {:else if phase === 'success' && config}
              <div class="alert alert-success success-state">
                <span class="glyphicon glyphicon-ok" aria-hidden="true"></span>
                <h2 class="section-title">{config.messages.success}</h2>
                <strong class="created-name">{createdUsername}</strong>
              </div>
            {/if}
          </div>
        </section>
      </div>
    </div>
  </div>
  <footer class="page-footer">
    <div class="container">
      <ul class="list-inline">
        <li>
          <a href="https://forge.asnk.io/sugar/nekomonogatari-bot" target="_blank" rel="noreferrer">
            {themeCopy.source}
          </a>
        </li>
        <li>
          <a href="https://forge.asnk.io/sugar/nekomonogatari-bot/src/branch/main/LICENSE" target="_blank" rel="noreferrer">
            AGPL-3.0-only
          </a>
        </li>
      </ul>
    </div>
  </footer>
</main>
