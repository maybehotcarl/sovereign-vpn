# $TDH Allocation Summary

## Scope

This is a proposed first-90-days allocation plan for `$TDH` rewards connected to 6529 VPN launch activity.

The conservative tokenomics cap first-90-days emissions at `0.5%` of the `100,000,000 TDH` max supply, or `500,000 TDH` total if the full cap is used. The cross-site planning target reserves up to `25%` of that Phase 0 budget for 6529 VPN node operation, or `125,000 TDH`.

This plan treats `125,000 TDH` as a ceiling, not a target. Unused tokens should remain unissued.

## Recommended 90-Day Budget

| Bucket | Share | Max TDH | Purpose |
| --- | ---: | ---: | --- |
| Node operator service rewards | 60% | 75,000 | Reward real VPN utility: uptime, served paid sessions, quality, and region coverage. |
| Node operator onboarding | 12% | 15,000 | Help bootstrap the supply side without rewarding registration alone. |
| User onboarding rewards | 10% | 12,500 | Small first-use rewards for meaningful completed user actions. |
| Quality, security, and abuse reporting | 8% | 10,000 | Reward verified reports and useful reliability feedback. |
| Ecosystem/integration grants | 5% | 6,250 | Support dashboards, node tooling, docs, monitoring, and ZK/access integrations. |
| Manual review and reserve | 5% | 6,250 | Buffer for edge cases, appeals, undercounted useful work, or skipped emissions. |
| **Total ceiling** | **100%** | **125,000** |  |

## Bucket Details

### 1. Node Operator Service Rewards - 75,000 TDH

Primary reward source. Operators should earn for measurable service, not merely for existing.

Suggested point inputs:

- Verified uptime during the epoch.
- Successful heartbeats plus independent external probes.
- Paid sessions served.
- Completed subscription/session time served.
- Aggregate bandwidth served, if measured in a privacy-preserving way.
- Low failed-connect and early-disconnect rates.
- Region scarcity multiplier for under-served regions.

Suggested exclusions:

- Overdue heartbeat windows.
- Nodes that fail external probes.
- Self-use or clearly circular usage.
- Nodes with unresolved abuse reports.
- Nodes that violate privacy/logging rules.

### 2. Node Operator Onboarding - 15,000 TDH

Small, controlled bootstrap rewards for getting reliable nodes online.

Eligible actions:

- First registered node that passes stake, card, endpoint, and policy checks.
- First registered RAILGUN payout address.
- First full 7-day healthy operating window.

Do not pay this on registration alone. Require a minimum healthy window first.

### 3. User Onboarding Rewards - 12,500 TDH

User rewards should be small because user activity is easier to farm.

Eligible actions:

- First paid subscription completed.
- First `$TDH`-paid subscription completed.
- First anonymous paid entitlement used successfully, if the user opts into a reward claim path.

Rules:

- One first-use reward per wallet per product.
- No reward for wallet connect alone.
- No reward for raw reconnects.
- No automatic reward just for paying a service fee.

### 4. Quality, Security, and Abuse Reporting - 10,000 TDH

Rewards for activity that improves the network.

Eligible actions:

- Valid bug report leading to a merged fix.
- Valid abuse report leading to enforcement.
- Useful post-session quality report that can be cross-checked against aggregate gateway/node data.
- Reproducible node reliability report.

Rules:

- Cap per wallet.
- Manual review required.
- Do not reward public reviews or promotion unless disclosure/compliance is handled separately.

### 5. Ecosystem And Integration Grants - 6,250 TDH

Small grants for launch-critical work around the VPN.

Examples:

- Node operator setup improvements.
- Monitoring dashboards.
- Privacy-preserving usage accounting.
- ZK entitlement issuer/indexer integration.
- Documentation that reduces operator support burden.
- Client tooling improvements.

### 6. Manual Review And Reserve - 6,250 TDH

Hold back a reserve instead of forcing full emissions.

Use cases:

- Correcting undercounted legitimate operator service.
- Handling appeal outcomes.
- Funding a small emergency incentive for under-served regions.
- Rolling unused rewards into nothing, rather than automatically increasing later emissions.

## Suggested Epoch Cadence

Use weekly epochs for the first 90 days:

- 13 weekly epochs.
- Maximum average weekly VPN allocation: about `9,615 TDH`.
- Publish each epoch only after review.
- Include metadata with totals, included events, excluded events, and reason codes.

The first two weeks should use lower emissions while the accounting pipeline is being checked. A reasonable ramp:

| Period | Max Weekly Allocation |
| --- | ---: |
| Weeks 1-2 | 5,000 TDH/week |
| Weeks 3-6 | 8,000 TDH/week |
| Weeks 7-13 | Up to remaining weekly cap |

## Reward Principles

- Reward completed utility, not intent.
- Reward operators more than users.
- Keep user first-use rewards small and capped.
- Keep payments and rewards separate.
- Do not make `$TDH` fees automatically generate `$TDH` rewards.
- Prefer points first, claimable tokens only after epoch review.
- Treat every published reward root as an auditable accounting artifact.

## Privacy Requirements

The VPN reward ledger must not store source IPs, destination IPs, traffic content, session tokens, full WireGuard keys, assigned tunnel IPs, or long-lived raw operational logs.

Use coarse activity records:

- operator address;
- role;
- event type;
- epoch window;
- duration bucket;
- paid/free/TDH-paid class;
- aggregate quality score;
- exclusion reason, if excluded.

For anonymous paid access, rewards and operator settlement should use aggregate accounting rather than public per-user session records.

## Recommended Initial Formula

Use reward points, then convert points to token amounts inside each epoch:

```text
operator_points =
  uptime_hours * uptime_multiplier
  + paid_session_hours_served * paid_session_multiplier
  + quality_bonus_points
  + scarcity_bonus_points
```

```text
account_reward =
  account_points / total_epoch_points * epoch_tdh_budget
```

Suggested starting multipliers:

- Uptime hour: `1 point`
- Paid session hour served: `3 points`
- TDH-paid session hour served: `4 points`
- Healthy 7-day onboarding completion: fixed `250 points`, once
- First paid user subscription completion: fixed `25 points`, once
- First TDH-paid user subscription completion: fixed `50 points`, once

These are points, not token promises. The epoch budget determines the actual claimable amount.

## Things Not To Reward

- Wallet connection.
- Raw VPN connects.
- Reconnect loops.
- User bandwidth consumption by itself.
- Holding `$TDH`.
- Paying a `$TDH` fee by itself.
- Self-dealing between a user wallet and its own node.
- Low-quality nodes that are merely online.
- Social posts, reviews, or referrals without a completed downstream action.

## Activation Checklist

- Implement a privacy-preserving VPN activity export.
- Add review and exclusion reason codes.
- Build a dry-run epoch before any live claim root.
- Confirm the first 90-day VPN cap with governance/admin.
- Complete legal and tax review before claims open.
- Publish durable epoch metadata before calling `publishEpoch`.
- Keep unused emissions unminted.
