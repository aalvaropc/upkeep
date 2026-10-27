# Deploying the demo

The demo is one binary behind Caddy on a single Ubuntu VM. Builds and tests of the sample pull requests run in Docker on the same machine, so the VM needs Docker and about 8 GB of memory: four pull requests are worked at once, each in a container limited to one CPU and 768 MB.

Any provider works. The demo only needs Token Factory for its models, which it reaches over the network.

## What to create

- Ubuntu 24.04, 2 vCPU, 8 GB of memory, 30 GB of disk, a public IPv4 address.
- Inbound TCP 22, 80 and 443 open.
- `cloud-init.yaml` as the instance's user data. It installs Docker and Caddy, creates the `upkeep` service user, pulls the sandbox image and writes `/etc/upkeep/upkeep.env` in replay mode. Add your SSH key to a sudo-capable user as your provider does it.

## Deploy

From a clean checkout of `main`:

```sh
deploy/deploy.sh admin@203.0.113.7
```

It builds the dashboard and a static linux/amd64 binary, copies them with the systemd unit and the Caddyfile, switches `/opt/upkeep/current` to the new release, restarts the service and waits for `/healthz`. If the release does not become healthy it switches back by itself. The host name defaults to `203-0-113-7.sslip.io`, which Caddy gets a certificate for; pass a second argument to use your own name.

Roll back to the previous release:

```sh
deploy/deploy.sh --rollback admin@203.0.113.7
```

## Switch to live models

Edit `/etc/upkeep/upkeep.env` on the VM, set `UPKEEP_MODE=live` and `NEBIUS_API_KEY` (and `TAVILY_API_KEY` for migration guides), then `sudo systemctl restart upkeep`. `deploy/upkeep.env.example` lists the variables. `UPKEEP_DAILY_TOKENS` caps live spending per day; past it runs replay the recording and say so. Before each live run the server makes a one-token check call to Token Factory, and if that fails the run replays the recording too, so a spent credit or a revoked key shows a notice instead of failed pull requests.

## Warm the build cache

The first run downloads Go modules for the sample into `/var/lib/upkeep/cache`. Warm it once so the first visitor does not wait:

```sh
sudo -u upkeep env UPKEEP_MODE=replay UPKEEP_SANDBOX=docker UPKEEP_DATA_DIR=/var/lib/upkeep /opt/upkeep/current/upkeep eval -runs 1 -out /tmp/warm
```

## Check it

```sh
curl -fsS https://203-0-113-7.sslip.io/healthz
curl -N https://203-0-113-7.sslip.io/api/sessions/none/events
```

The second call answers 404 for an unknown session, which shows the API is reachable; open the page and press the button to see the event stream through Caddy.

## What the service may touch

The unit runs as the `upkeep` user with no new privileges, a read-only file system except `/var/lib/upkeep`, a private `/tmp` and no access to home directories. Membership of the `docker` group gives that user the power of the Docker daemon, which is how it starts the sandbox containers; the containers themselves run without network, without capabilities and with a read-only root.
