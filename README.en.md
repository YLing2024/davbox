[English](README.en.md) | [简体中文](README.md)

# davbox

A self-hosted WebDAV service with only two concepts: **admin manages accounts, client logs in and manages files**.
Aimed at apps that need WebDAV sync (SiYuan, Joplin, etc.): one app, one account, one directory, invisible to each other.

![License](https://img.shields.io/badge/license-MIT-blue)

## Why build it yourself

Open-source WebDAV services on the market are either operations platforms (SFTPGo: a dozen concepts, an all-English admin UI), or have only startup flags and no admin interface (dufs), or have isolation that is not trustworthy in practice (hacdias/webdav).
The gap needed is only a thin layer: **minimal account management + trustworthy directory isolation**. The protocol layer is not rewritten; the Go official implementation is used.

## Features

- **One app, one account, one directory**: each account sees only its own root directory, and cross-account access is always denied; path escapes (`../`, encoded `%2e`, backslashes, null bytes) are always `400`
- **Admin page**: add / disable / delete accounts, change passwords, view each account's directory and usage, and copy connection info (address + username + password) in one click
- **Client page**: after logging in, browse / upload (including drag and drop) / download / rename / create folder / delete
- **Read-only accounts**: all write verbs are `403`, reads work normally
- **Official protocol implementation**: `golang.org/x/net/webdav` (including `LOCK` / `UNLOCK` / `COPY` / `MOVE` / dead property persistence), not a home-grown protocol
- **Single-binary delivery**: the frontend is embedded into the binary with `//go:embed`, with no external runtime and no database
- **Switchable admin authentication**: built-in admin password by default, or disable it and take identity from an upstream gateway

## Quick start

```bash
git clone https://github.com/YLing2024/davbox.git
cd davbox
make build        # build the frontend first, then compile the single binary ./davbox
./davbox          # listens on 127.0.0.1:18900 by default, data under ./data
```

On first start (`builtin` mode) it generates in the data directory:

- `accounts.json` — the account table
- `admin.json` — bcrypt hash of the admin password
- `admin-password.txt` — the admin's initial plaintext password
- `secret.key` — the signing key for the session cookie

Saving settings (such as the CORS allow-list) on the admin page writes `settings.json` (atomic replace).

The admin's initial password is printed to stdout only when first generated; save it immediately.

- Admin page `/admin`, file page `/`
- For the app's WebDAV address, enter `http(s)://<your site>/<app name>`

Startup options:

| Option | Default | Description |
|---|---|---|
| `-addr` | `127.0.0.1:18900` | listen address. Listening on loopback only is recommended, with nginx providing TLS and the entry point |
| `-data` | `./data` | data directory (account table, admin credentials, session key, per-account directories) |

Other Makefile targets: `make frontend` builds only the frontend, `make run` starts directly after compiling, `make vet` runs static checks, `make test` runs tests, `make clean` cleans artifacts.

## Directory structure

```
cmd/davbox/     program entry
internal/       accounts, auth and sessions, plus the WebDAV and admin/client routes
web/            Vite + React + TS frontend (build output embedded via go:embed)
docs/           requirements, technology choices and acceptance records
```

## Configuration

| Name | Default | Description |
|---|---|---|
| `AUTH_MODE` | `builtin` | Admin authentication mode, either `builtin` or `sso` |
| `CORS_ORIGINS` | (empty, off) | First-run default for the CORS allow-list, comma-separated, exact match on `scheme://host[:port]`; maintain it from the "Cross-origin allow-list (CORS)" card on the admin page |

- `builtin`: built-in admin password; log in at `/admin` by entering it, using a signed cookie session (12 hours).
- `sso`: no built-in password login; the admin identity comes from the `X-Auth-User` injected by the gateway; a missing or empty value returns `401 JSON` and does not fall back to a cookie. Use it only when davbox listens on loopback only and the header is injected by the gateway and stripped from the outside.

```bash
AUTH_MODE=builtin ./davbox -addr 127.0.0.1:18900 -data ./data   # default, can be omitted
AUTH_MODE=sso     ./davbox -addr 127.0.0.1:18900 -data ./data
```

What is unaffected in both modes:

- WebDAV data plane `/<account name>/...`: always uses the app account + HTTP Basic
- Client endpoints `/api/client/*` and static assets `/assets/*`
- Account CRUD always requires an admin identity in both modes

## Direct browser access (CORS)

Off by default. When a browser uses `fetch` to talk to WebDAV directly, methods such as `PROPFIND` / `PUT` trigger a CORS preflight; without CORS the preflight gets `401` and the browser only reports `TypeError: Failed to fetch`.

Configure it in the "Cross-origin allow-list (CORS)" card on the admin page `/admin`: one origin per line (comma-separated also works). A save takes effect immediately, no restart; an empty list turns CORS off.

- Only `scheme://host` or `scheme://host:port` is accepted, with `scheme` limited to `http` / `https`; an invalid entry is reported on the spot with its line number. A trailing `/` is ignored, `scheme`/`host` are case-insensitive, the port is exact (`https://app.example.com` and `https://app.example.com:443` are different origins).
- The settings live in `settings.json` in the data directory; the `CORS_ORIGINS` environment variable is only the **first-run default**, and once you save on the admin page the page value wins.
- A matching preflight returns `204` without authentication; the actual request afterwards still uses account + Basic, so permissions and directory isolation are unchanged.
- Only list frontend origins you trust, never `*` and never someone else's site. Cross-origin responses allow credentials (`Access-Control-Allow-Credentials: true`), so a wrong origin hands your WebDAV to it.

**Alternative**: if you would rather not enable CORS (or the upstream WebDAV server cannot), put the frontend and WebDAV on the same origin and reverse-proxy WebDAV under a sub-path with nginx; the frontend then uses a same-origin path and needs no CORS at all:

```nginx
server {
    listen 443 ssl;
    server_name app.example.com;

    # frontend assets / SPA
    location / {
        root /var/www/app;
        try_files $uri /index.html;
    }

    # same-origin sub-path straight to davbox's WebDAV data plane
    location /dav/ {
        proxy_pass http://127.0.0.1:18900/;
        proxy_request_buffering off;
        client_max_body_size 0;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Point the frontend's WebDAV address at the same-origin path `/dav/<app name>` (e.g. `/dav/myapp`). Third-party WebDAV servers often cannot enable CORS, in which case the same-origin proxy is the only workable option.

## Deployment (nginx reverse proxy)

```nginx
server {
    listen 443 ssl;
    server_name dav.example.com;

    location / {
        proxy_pass http://127.0.0.1:18900;
        proxy_request_buffering off;   # stream WebDAV request bodies, no large-file staging
        client_max_body_size 0;        # no size limit
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

For `sso`, hand `/admin` and `/api/admin/` to your auth entry point, and connect the WebDAV data plane and `/assets/*` directly to davbox — the protocol endpoints keep their own Basic authentication, otherwise apps cannot sync.

## Done

- Account isolation and the six WebDAV verbs: `GET` / `PUT` / `DELETE` / `MKCOL` / `MOVE` / `PROPFIND` (`Depth 0/1` both return valid `207`)
- Admin page and client page
- Single binary + nginx reverse proxy deployment

## License

[MIT](LICENSE)
