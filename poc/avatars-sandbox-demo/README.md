# Demo: `isolation: sandbox` — a compose service in a Docker Sandbox

This is [dockersamples/avatars](https://github.com/dockersamples/avatars)
with two changes:

- the `api` service declares `isolation: sandbox`: compose runs it in a
  Docker Sandbox (microVM, no internet access) instead of the engine
- the `web` dev-server proxy target is configurable (`API_URL`), pointing at
  the sandboxed api through its published port (`http://api:5734`)

## Requirements

- Docker Desktop, and the `sbx` CLI (Docker Sandboxes) in PATH
- the compose binary built from this branch (`make build`)

## Run

```bash
cd poc/avatars-sandbox-demo
../../bin/build/docker-compose up -d --build
```

- `docker ps` shows only `avatars-web-1`; `sbx ls` shows the `avatars-api`
  sandbox with `127.0.0.1:5734->80/tcp`
- open http://localhost:5735 — avatars are served by the sandboxed api
  through the web proxy (`web -> host-gateway -> sandbox`)
- the sandbox has no egress:
  `sbx exec avatars-api python3 -c "import urllib.request;urllib.request.urlopen('https://example.com')"`
  fails with 403 from the policy proxy

## Tear down

```bash
../../bin/build/docker-compose down
```

removes the web container, the network, and the sandbox.
