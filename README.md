# goravel-ship 🚢

Zero-config Docker deployment package for [Goravel](https://goravel.dev) framework.

Build your Docker image locally and ship it to any server via SSH — no Docker Hub, no CI/CD setup required.

---

## Installation

```bash
go get github.com/president-tuychiyev/goravel-ship
```

Register the service provider in `bootstrap/providers.go`:

```go
import ship "github.com/president-tuychiyev/goravel-ship"

func Providers() []foundation.ServiceProvider {
    return []foundation.ServiceProvider{
        // ... existing providers
        &ship.ServiceProvider{},
    }
}
```

Scaffold required files into your project:

```bash
go run . artisan ship:install
```

This creates:
- `docker-compose-prod.yml` — production compose config
- `.env.prod.example` — production env template
- `deploy.sh` — remote deployment script

Edit `.env.prod.example` with your production values, then you're ready to ship.

---

## Usage

```bash
go run . artisan ship --user=<user> --ip=<server-ip> [flags]
```

### Flags

| Flag          | Alias | Default                | Required |
|---------------|-------|------------------------|----------|
| `--user`      | `-u`  | —                      | ✅        |
| `--ip`        | —     | —                      | ✅        |
| `--port`      | `-P`  | `22`                   | ❌        |
| `--path`      | `-p`  | `/opt/app`             | ❌        |
| `--tag`       | `-t`  | `latest`               | ❌        |
| `--image`     | `-i`  | *(auto from go.mod)*   | ❌        |
| `--container` | `-c`  | *(same as image name)* | ❌        |
| `--binary`    | `-b`  | `/usr/local/bin/app`   | ❌        |
| `--migrate`   | `-m`  | `false`                | ❌        |
| `--fresh`     | —     | `false`                | ❌        |
| `--seed`      | `-s`  | `false`                | ❌        |

> `--fresh` only works together with `--migrate`. It runs `migrate:fresh` instead of `migrate`.

### Examples

**Minimal:**
```bash
go run . artisan ship -u root --ip=192.168.1.100
```

**Custom port and path:**
```bash
go run . artisan ship -u deploy --ip=1.2.3.4 -P 2222 -p /home/deploy/myapp
```

**With version tag:**
```bash
go run . artisan ship -u root --ip=1.2.3.4 -t v1.2.0
```

**Deploy + migrate:**
```bash
go run . artisan ship -u root --ip=1.2.3.4 -m
```

**Deploy + migrate:fresh:**
```bash
go run . artisan ship -u root --ip=1.2.3.4 -m --fresh
```

**Deploy + migrate:fresh + seed:**
```bash
go run . artisan ship -u root --ip=1.2.3.4 -m --fresh -s
```

**Deploy + migrate + seed:**
```bash
go run . artisan ship -u root --ip=1.2.3.4 -m -s
```

**Full example:**
```bash
go run . artisan ship -u deploy --ip=1.2.3.4 -P 2222 -p /home/deploy/app -c my_container -t v2.0.0 -m -s
```

> **Git Bash (MINGW64) note:** Prepend `MSYS_NO_PATHCONV=1` to prevent path conversion:
> ```bash
> MSYS_NO_PATHCONV=1 go run . artisan ship -u root --ip=1.2.3.4 -p /home/deploy/app
> ```

---

## How it works

1. Rebuilds `.env` from `.env.prod.example` on every deploy (always up to date)
2. Runs `docker build` to create the image
3. Saves the image as `<sha256-hash>.tar.gz` (unguessable filename)
4. Opens a single SSH tunnel — **password asked only once**
5. Uploads via SCP: `tar.gz`, `.env`, `docker-compose-prod.yml` (→ `docker-compose.yml` on server), `deploy.sh`
6. Runs `deploy.sh` on the server:
   - Loads the image via `docker load` — no Docker Hub needed
   - Passes `IMAGE_NAME` env so compose uses the correct local image
   - Recreates the container via `docker compose up -d`
   - Prunes dangling images
   - Cleans up `tar.gz` and self-deletes `deploy.sh`
   - On error: also removes `docker-compose.yml` for a clean state
7. If `--migrate`: runs `artisan migrate` inside the container
8. If `--migrate --fresh`: runs `artisan migrate:fresh` instead
9. If `--seed`: runs `artisan db:seed` inside the container
10. Removes local `tar.gz`

---

## Configuration tips

### Container name

By default `--container` equals `--image` (auto-detected from `go.mod`). If your `docker-compose.yml` uses a different `container_name`, pass it explicitly:

```bash
go run . artisan ship -u root --ip=1.2.3.4 --container my_app_container -m
```

### Binary path

The `--binary` flag must match the path of the compiled binary inside the container. Default is `/usr/local/bin/app`. Update your `Dockerfile` or pass the correct path:

```bash
go run . artisan ship -u root --ip=1.2.3.4 --binary /usr/local/bin/my_app -m
```

---

## Server requirements

- Docker + Docker Compose v2
- User must be in the `docker` group (run once):
  ```bash
  sudo usermod -aG docker <username>
  newgrp docker
  ```

---

## License

MIT
