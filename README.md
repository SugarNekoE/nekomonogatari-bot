# nekomonogatari-bot

A small, plugin-based Telegram group bot with three optional integrations:

- `asnk-forge`: authorizes a Telegram group member in a short-lived web flow and creates that member's Forgejo account through the administrator API.
- `mc-whitelist`: lets each Telegram account manage up to two Minecraft Java Edition whitelist names through RCON.
- `system-status`: reports the bot host's operating system, hardware, utilization, load, storage, virtualization, and Go runtime.

The bot uses GORM with the pure-Go `github.com/glebarez/sqlite` dialector, so it builds with `CGO_ENABLED=0`. Plugins that persist data share one database connection while owning their quoted, plugin-prefixed tables (for example, `"mc-whitelist_players"`).

## Configuration

Copy [`config.example.yaml`](config.example.yaml) to `config.yaml`, fill in the secrets, and add the Telegram group chat IDs that may use the bot. An empty `telegram.allowed-groups` list intentionally disables commands in every chat.

```bash
cp config.example.yaml config.yaml
go run .
```

`CONFIG_PATH` can point directly to `config.yaml`. For compatibility it may also point to a directory containing `config.yaml`. Every setting can also be supplied as an environment variable by replacing dots and hyphens with underscores, for example:

```text
NEKOMONOGATARI_LANGUAGE
TELEGRAM_TOKEN
TELEGRAM_ALLOWED_GROUPS
DATABASE_PATH
PLUGINS_ASNK_FORGE_ENABLED
PLUGINS_ASNK_FORGE_FORGEJO_API_TOKEN
PLUGINS_MC_WHITELIST_ENABLED
PLUGINS_MC_WHITELIST_PASSWORD
PLUGINS_SYSTEM_STATUS_ENABLED
PLUGINS_SYSTEM_STATUS_DISK_PATH
PLUGINS_SYSTEM_STATUS_SHOW_HOSTNAME
PLUGINS_SYSTEM_STATUS_TIMEOUT
```

The global `language` setting accepts `en` or `zh` and controls both plugins, including the Forge registration site. Its environment variable is namespaced as `NEKOMONOGATARI_LANGUAGE` so the operating system's locale `LANGUAGE` variable cannot override it.

> > > > > > > 643d3ab (feat(flake): package this project as a nixpkg)

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

## System status

Enable `plugins.system-status` to provide `/status` in allowed groups. The response includes the operating system and kernel, architecture, host uptime, CPU model and topology, CPU utilization, load averages, memory, the configured filesystem usage, virtualization details when available, and the bot's Go runtime.

`disk-path` selects the filesystem to report. `show-hostname` is disabled by default to avoid publishing the server hostname into a group chat. Collection is bounded by `timeout`; individual unavailable metrics are omitted while the remaining status is still returned. Network addresses, environment variables, process arguments, and secrets are never included.

## Nix package and NixOS service

The flake exports `packages.<system>.default`, an overlay, and `nixosModules.default`. Build the package with:

```bash
nix build
```

A NixOS configuration can enable the hardened systemd service as follows:

```nix
{
  inputs.nekomonogatari-bot.url = "git+https://forge.asnk.io/sugar/nekomonogatari-bot.git";

  outputs = inputs@{ nixpkgs, nekomonogatari-bot, ... }: {
    nixosConfigurations.my-host = nixpkgs.lib.nixosSystem {
      modules = [
        nekomonogatari-bot.nixosModules.default
        {
          services.nekomonogatari-bot = {
            enable = true;
            # Keep secrets out of the Nix store. The service user must be able
            # to read this file (for example via sops-nix owner/group options).
            configPath = "/run/secrets/nekomonogatari-bot.yaml";
          };
        }
      ];
    };
  };
}
```

Relative database paths are stored under `/var/lib/nekomonogatari-bot`. The service runs as the dedicated `nekomonogatari-bot` user. Do not set `configPath = ./config.yaml` when it contains secrets, because a Nix path literal is copied to the world-readable Nix store.

The Forgejo workflow in [`.forgejo/workflows/nix.yaml`](.forgejo/workflows/nix.yaml) expects a `CACHIX_CACHE_NAME` repository variable and `CACHIX_AUTH_TOKEN` repository secret.

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
