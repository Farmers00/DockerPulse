# DockPulse (DockMgr)

> **Multi-Server Docker & Compose Fleet Manager with Native Host Directory Support**

DockPulse is a high-performance, lightweight fleet management dashboard for Docker environments. It was engineered specifically to solve Portainer's limitations around custom host directory structures (like `~/docker/<stack>/docker-compose.yml`), making multi-server Docker ops effortless without needing to constantly SSH into your Linux nodes.

---

## 🌟 Why DockPulse?

- **Native Host Directory Tracking**: Portainer forces stacks into its internal storage. DockPulse directly monitors and respects your server's native directories (e.g. `~/docker/nextcloud/docker-compose.yml`).
- **1-Click Push-Button Updates**: Registry digest comparison detects image updates remotely. Click **"Pull & Up"** to execute `docker compose pull && docker compose up -d` with a real-time streaming terminal drawer.
- **In-Browser Compose & .env Editor**: Edit `docker-compose.yml` and `.env` in the browser with **automatic revision snapshots** prior to every save—enabling 1-click rollback if a configuration breaks.
- **Tri-Mode Connection Drivers**:
  1. **DockPulse Agent**: Minimal agent container on remote nodes communicating over secure, persistent WebSockets.
  2. **Direct SSH**: Zero-install management using OpenSSH keypairs.
  3. **Docker Socket / TLS**: Direct TCP or unix socket connectivity.
- **Live Observability**: Live log streaming with follow (`-f`), regex search, tail limits, and interactive container terminal PTY shell (`xterm.js`).
- **Network & Storage Ops**: Inspect IPAM subnets and gateways, attach/detach containers, and run safe Docker prune wizards.
- **Ultra-Lightweight**: Built as a single Go binary with embedded SQLite and React UI. Uses <40MB of RAM.

---

## 🚀 Quick Start

### 1. Run DockPulse Central Server

Run with Docker Compose:

```yaml
services:
  dockpulse:
    image: dockpulse/dockmgr:latest
    container_name: dockpulse
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - ./data:/data
      - /var/run/docker.sock:/var/run/docker.sock
      - ${HOME}/docker:/root/docker
    environment:
      - PORT=8080
      - DATA_DIR=/data
      - JWT_SECRET=change_to_a_secure_random_key
      - AGENT_SECRET=dockpulse_agent_shared_token
      - PROXY_AUTH_HEADER=Remote-User # Optional for Authelia/Authentik
```

Start the container:
```bash
docker compose up -d
```
Then open `http://<your-server-ip>:8080` to complete the first-time admin setup wizard.

---

### 2. Connect Remote Hosts

#### Option A: Lightweight Agent (Recommended)
On your remote Linux host, deploy `docker-compose.agent.yml`:

```yaml
services:
  dockpulse-agent:
    image: dockpulse/dockmgr:latest
    container_name: dockpulse-agent
    restart: unless-stopped
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ${HOME}/docker:/root/docker
    command: >
      dockmgr agent
      --server ws://manager-ip:8080/ws/agent
      --token dockpulse_agent_shared_token
      --host-id node-1
      --base-dir /root/docker
```

#### Option B: Direct SSH
In the DockPulse web dashboard:
1. Click **Add Server** -> **Direct SSH**.
2. Enter the host IP, SSH port, user, and paste your SSH Private Key.
3. DockPulse will immediately connect and scan `~/docker` for stacks.

---

## 🛠️ Project Architecture

```
docker_mgr/
├── backend/
│   ├── cmd/dockmgr/main.go          # Single CLI binary: server or agent
│   ├── internal/
│   │   ├── api/                     # REST, WebSockets, streaming handlers
│   │   ├── auth/                    # JWT, bcrypt, reverse proxy header SSO
│   │   ├── config/                  # Configuration loader
│   │   ├── database/                # SQLite embedded repository & models
│   │   ├── driver/                  # HostDriver interface (Agent, SSH, Socket)
│   │   ├── scanner/                 # Filesystem stack discovery (~/docker)
│   │   ├── updater/                 # Registry manifest digest comparison engine
│   │   └── agent/                   # Agent node worker implementation
├── frontend/
│   ├── src/
│   │   ├── api/client.ts            # Typed REST & WebSocket client
│   │   ├── components/
│   │   │   ├── TerminalModal.tsx    # xterm.js PTY shell
│   │   │   ├── LiveLogsModal.tsx    # Streaming logs with follow (-f)
│   │   │   ├── ComposeEditorModal.tsx # In-browser editor & revision rollback
│   │   │   ├── UpdateModal.tsx      # Push-button update stream drawer
│   │   │   ├── NetworksModal.tsx    # IPAM inspection & network controls
│   │   │   ├── StorageModal.tsx     # Disk usage & prune wizard
│   │   │   └── AddHostModal.tsx     # Host onboarding wizard
│   │   └── App.tsx                  # Fleet overview dashboard
├── deploy/
│   ├── docker-compose.yml           # Central manager compose
│   ├── docker-compose.agent.yml     # Remote node agent compose
│   └── Dockerfile                   # Multi-stage production build
└── README.md
```

---

## 🔒 Security & SSO

- **Local Admin Setup**: First-run setup wizard creates the primary administrator account using bcrypt-hashed passwords and signed JWT tokens.
- **Reverse Proxy Header Authentication**: Compatible out of the box with Authelia, Authentik, and Cloudflare Access (`Remote-User` / `Remote-Email`).
- **No-Root Required for SSH**: Use any user that is in the remote host's `docker` group.

---

## 📄 License
MIT License
