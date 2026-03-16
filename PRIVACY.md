# Privacy And Logging

## Raw Operational Logs

Sovereign VPN should not retain raw operational logs for more than `1 hour`.

Raw operational logs include gateway stdout/stderr, reverse-proxy access logs, and any container or host logs that can be correlated to individual user activity.

Raw operational logs must never contain:

- Source IP addresses
- Destination IPs, DNS queries, or traffic content
- Full wallet addresses for end users
- Session tokens or SIWE signatures
- Full WireGuard public keys
- Assigned client tunnel IPs

Raw operational logs may contain only the minimum needed for live troubleshooting, such as:

- Aggregate counters
- Tier labels
- Durations
- Contract addresses
- Transaction hashes for operator/governance actions
- Coarse error categories

## Deployment Requirements

The app code redacts sensitive runtime logs, but log retention is ultimately an infrastructure control.

- Do not enable reverse-proxy access logs unless they are redacted and purged within `1 hour`.
- If reverse-proxy access logs are enabled, ensure token-bearing headers such as `Authorization` are redacted before logs are stored.
- Docker's built-in log rotation is size-based, not time-based. It reduces disk persistence, but it does not satisfy a strict `1 hour` retention requirement on its own.
- To meet the `1 hour` policy, run logs through infrastructure that enforces TTL-based deletion, or disable persistent raw container logs entirely.

## Known Privacy Limits

- Node operators can still observe user traffic as part of normal VPN operation.
- The access gateway still sees wallet addresses during authentication.
- On-chain session metadata is public when session contracts are used.

## Target Architecture

The current implementation is not the final privacy architecture. The target anonymous admission design is documented in [ANONYMOUS_ACCESS_PROTOCOL.md](ANONYMOUS_ACCESS_PROTOCOL.md), and the paid-anonymous launch variant is documented in [PAID_ANON_ACCESS_V1.md](PAID_ANON_ACCESS_V1.md).

This project should be described as using minimal, short-lived off-chain logs, not as a literal "no logs" VPN.
