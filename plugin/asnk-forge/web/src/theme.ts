const root = document.documentElement;
let preference = 'auto';

try {
  const stored = localStorage.getItem('asnk-forge-theme');
  if (stored === 'auto' || stored === 'light' || stored === 'dark') preference = stored;
} catch {}

const resolved = preference === 'auto'
  ? matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  : preference;

root.dataset.theme = resolved;
root.dataset.themePreference = preference;
root.style.colorScheme = resolved;
