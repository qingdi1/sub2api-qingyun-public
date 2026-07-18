# Qingyun Docker Update Agent

The Qingyun in-app update endpoint delegates to a separate internal container,
not to the main Sub2API application. The supported deployment baseline is
[`docker-compose.qingyun.yml`](docker-compose.qingyun.yml) in this repository.

## Published Images

Pin the application and agent to the same Qingyun release version. For example:

```text
SUB2API_IMAGE=ghcr.io/qingdi1/sub2api-qingyun-public:0.1.158-qingyun.3
UPDATE_AGENT_IMAGE=ghcr.io/qingdi1/sub2api-qingyun-update-agent:0.1.158-qingyun.3
```

Do not use `latest` for either image. The agent has Docker socket access and
must remain a known, reviewed companion to the application release.

## Contract

The backend calls `POST /v1/update` on `UPDATE_DOCKER_AGENT_URL` with:

```json
{"target_version":"0.1.158-qingyun.3"}
```

The request requires `Authorization: Bearer <UPDATE_DOCKER_AGENT_TOKEN>`.
The accepted response is:

```json
{"queued":true,"target_version":"0.1.158-qingyun.3","message":"..."}
```

Rollback uses the same authenticated payload and safety checks at
`POST /v1/rollback`. The backend only sends a version returned by the public
repository's rollback allowlist; the agent still resolves it to the fixed
Qingyun GHCR image and reports an asynchronous `queued` result.

The agent accepts only release-version strings. It always pulls the fixed image
repository `ghcr.io/qingdi1/sub2api-qingyun-public:<version>` and rejects an
image unless its OCI source and version labels match the Qingyun public
repository and requested version.

## Safety Boundary

- Only a container with both `io.qingyun.sub2api.update-target=true` and
  `io.qingyun.sub2api.component=sub2api` can be selected.
- It requires exactly one such container. Postgres and Redis never carry those
  labels and are never stopped, restarted, removed, or disconnected.
- The old application is renamed and disconnected from its networks only after
  the new release has been pulled and validated. The replacement receives a
  sanitized network configuration without inspect-only IDs, allocated IPs,
  gateways, or DNS names.
- The old application remains available for rollback until the replacement is
  running and Docker reports it `healthy`. Any failure stops/removes the new
  container, reconnects and renames the old one, then starts it again.
- The agent is internal-only on `sub2api-network`; do not publish its port.
  Its Docker socket access is intentionally isolated from the main application.

## Bootstrap And Source Test

The first managed deployment must be a manually selected, newly published
Qingyun image that already contains the backend update and rollback endpoint.
Do not bootstrap from a release older than `v0.1.158-qingyun.3`.

For a normal deployment, copy `.env.qingyun.example` to `.env.qingyun`, set
the local secrets, then run:

```bash
docker compose --env-file .env.qingyun -f deploy/docker-compose.qingyun.yml up -d
```

Only one Qingyun-managed application container may run on a Docker host. The
agent deliberately rejects ambiguous targets rather than risking an update to
the wrong application.

For an isolated local source test, copy `.env.qingyun.source.example` to
`.env.qingyun.source`, set its local secrets, then run:

```powershell
cd F:\Community\Sub2api
docker compose -p qingyun-update-source --env-file .env.qingyun.source -f docker-compose.qingyun.yml -f docker-compose.qingyun.source.yml up --build
```

This uses separate container names, port `8890`, and `*_qingyun_update_source`
data directories. It must not be run against or in place of the existing
`sub2api-ink` deployment.
