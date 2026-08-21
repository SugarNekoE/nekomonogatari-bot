# nekomonogatari-bot

A small, plugin-based Telegram group bot with two optional integrations:

- `asnk-forge`: authorizes a Telegram group member in a short-lived web flow and creates that member's Forgejo account through the administrator API.
- `mc-whitelist`: lets each Telegram account manage up to two Minecraft Java Edition whitelist names through RCON.

The bot uses GORM with the pure-Go `github.com/glebarez/sqlite` dialector, so it builds with `CGO_ENABLED=0`. Every plugin shares one database connection while owning its quoted, plugin-prefixed tables (for example, `"mc-whitelist_players"`).

## Configuration

Copy [`config.example.yaml`](config.example.yaml) to `config.yaml`, fill in the secrets, and add the Telegram group chat IDs that may use the bot. An empty `telegram.allowed-groups` list intentionally disables commands in every chat.

```bash
cp config.example.yaml config.yaml
go run .
```

`CONFIG_PATH` can point to the directory containing `config.yaml`. Every setting can also be supplied as an environment variable by replacing dots and hyphens with underscores, for example:

```text
TELEGRAM_TOKEN
TELEGRAM_ALLOWED_GROUPS
DATABASE_PATH
PLUGINS_ASNK_FORGE_ENABLED
PLUGINS_ASNK_FORGE_FORGEJO_API_TOKEN
PLUGINS_MC_WHITELIST_ENABLED
PLUGINS_MC_WHITELIST_PASSWORD
```

The global `language` setting accepts `en` or `zh` and controls both plugins, including the Forge registration site.

Each plugin has its own `enabled` switch. Disabled plugins do not migrate their schema, register handlers, or start services.

## Forge registration

Before enabling `asnk-forge`:

1. Set the bot's public registration domain with BotFather's `/setdomain` command.
2. Put that bot username in `telegram.username` without the leading `@`.
3. Configure a public HTTPS origin in `public-url`. Plain HTTP is accepted only for local development on a loopback host.
4. Create a Forgejo administrator API token and configure the Forgejo base URL. The token is used only by the Go backend and is never sent to the browser. HTTPS is required unless `allow-insecure-forgejo` is explicitly enabled for a trusted private network.
5. Reverse-proxy the public origin to `listen-address` when the service is not directly exposed.

In an allowed group, `/forge` explains the process and shows an authorization button. Telegram callback data lets the bot persist exactly which numeric Telegram account claimed the flow. The bot then posts the same public registration URL for everyone—there is no user ID, nonce, or bearer token in that URL. On the Svelte page, Telegram's Login Widget sends its signed identity to the backend, which verifies the signature and age and requires an unexpired claim for the same account. Only then is an HttpOnly session issued and the Forgejo form made available.

The registration page lives in `plugin/asnk-forge/web` and uses Svelte with Vite, without SvelteKit. The compiled assets are embedded in the Go binary. After changing the frontend, rebuild them with:

```bash
cd plugin/asnk-forge/web
npm ci
npm run build
```

## Minecraft whitelist

Enable RCON in the Minecraft server's `server.properties`, set a strong `rcon.password`, and configure the matching address/password in `plugins.mc-whitelist`. Keep RCON on a private network; it is not encrypted.

Available commands in an allowed group:

```text
/mcwl                     open the help and navigation menu
/mcwl add <player_name>  add and bind a Java Edition player name
/mcwl list               list your bindings and remaining capacity
/mcwl del <player_name>  remove one of your bindings
```

Player names must be 3–16 ASCII letters, digits, or underscores. Bindings are keyed by immutable Telegram numeric account ID, compared case-insensitively, and limited to two by SQLite as well as application checks.

## Development checks

```bash
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go vet ./...

cd plugin/asnk-forge/web
npm ci
npm run check
npm run build
```

Live end-to-end testing additionally needs a real Telegram bot and allowed group, a public HTTPS domain associated with that bot, a Forgejo administrator token, and a reachable Minecraft RCON server.
