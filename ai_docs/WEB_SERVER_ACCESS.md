# Web Server Access

The meroku web server (`meroku --web`, `--web-port`, and "web app" in the main menu)
serves the UI, `/api/*` and `/ws/*` from one Go router. The API reads and writes SSM
parameters, S3 objects and env YAML with the user's AWS credentials, so only the UI may
call it.

## Bind address

The server listens on `127.0.0.1` by default. `--listen <address>` chooses another
interface; `webListenAddr` (`app/web_access.go`) builds the address for the port check,
`--web-port` and the auto-port start paths.

## The guard

`mainRouter()` returns `webAccessGuard(mux)`, so every route, including routes added
later, passes one check, `webRequestAllowed`. A refused request gets `403 forbidden`
before any handler runs. The WebSocket upgraders (`app/api_logs.go`,
`app/api_ssh_pty.go`) use the same function as `CheckOrigin`, and the Neon handlers
repeat it.

Local mode (`--listen` is `127.0.0.1`, `localhost` or `::1`):

- `RemoteAddr` is a loopback address, which refuses other machines.
- `Host` is `localhost`, `127.0.0.1` or `[::1]`, on the port the request reached. A
  DNS-rebinding page in a local browser sends its own name as Host and is refused.
- `Origin`, when sent, is an `http` origin on a loopback name (any port, for the Vite dev
  server).
- `Sec-Fetch-Site`, when sent, is `same-origin` or `none`.

Clients that send no `Origin` or `Sec-Fetch-Site` (curl, scripts) pass from loopback.

Remote mode (any other `--listen`, e.g. `0.0.0.0`):

- meroku generates a 32-byte random token at start and prints
  `http://<address>:<port>/?token=<token>` once. Anyone with that URL has access.
- Opening the URL sets the `meroku_token` cookie (`HttpOnly`, `SameSite=Strict`,
  `Path=/`) and redirects (303) to the same path without the token.
- Every request carries the token, as the cookie or a `token` query parameter, compared
  in constant time.
- `Origin`, when sent, equals `http://<Host>`; `Sec-Fetch-Site` follows the local rule.
- Only a page navigation with the token query is redirected. A WebSocket upgrade or a
  request under `/api/` or `/ws/` with the token query goes straight to its handler. The
  redirect target is the cleaned path, so `//other.host/x` redirects to `/other.host/x`.
- `--listen` takes an address without a port (`--web-port` sets the port); meroku exits
  with an error for `0.0.0.0:8080` or a value that is neither an IP nor a hostname.

Remote mode is plain HTTP: the token crosses the network in clear text, so use it only on
a trusted network or through an SSH tunnel, and a TLS reverse proxy in front is refused
because its Origin is `https`.

## No CORS

No route sends `Access-Control-*` headers. The UI is same-origin: Go serves the built
bundle, and the Vite dev server proxies `/api` and `/ws`. A cross-origin preflight is
refused by the guard like any other foreign request.

## Vite dev server

`web/vite.config.ts` uses Vite's default host (localhost) and proxies to
`MEROKU_API_URL` (default `http://localhost:8080`) with `changeOrigin`, so meroku sees a
loopback Host and the dev server's loopback Origin, which local mode accepts. The
Playwright harness starts Vite with `--host 127.0.0.1`.

## Tests

`app/web_access_test.go` runs requests through `mainRouter()` in both modes.
