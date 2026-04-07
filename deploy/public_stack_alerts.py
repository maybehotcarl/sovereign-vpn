#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import os
import shlex
import subprocess
import sys
import time
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any
from urllib import error as urllib_error
from urllib import request as urllib_request


def now_iso() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def env_bool(name: str, default: bool) -> bool:
    raw = os.getenv(name)
    if raw is None or raw.strip() == "":
        return default
    return raw.strip().lower() in {"1", "true", "yes", "on"}


def env_int(name: str, default: int) -> int:
    raw = os.getenv(name)
    if raw is None or raw.strip() == "":
        return default
    try:
        value = int(raw.strip(), 10)
    except ValueError:
        return default
    return value if value > 0 else default


def trim_trailing_slash(value: str) -> str:
    return value[:-1] if value.endswith("/") else value


@dataclass
class Config:
    public_site_url: str
    remote_host: str | None
    service_name: str
    environment: str
    webhook_url: str | None
    webhook_bearer_token: str | None
    telegram_bot_token: str | None
    telegram_chat_id: str | None
    telegram_message_thread_id: str | None
    gateway_service_name: str
    zk_api_service_name: str
    wg_interface: str
    state_file: Path
    http_timeout_seconds: int
    command_timeout_seconds: int
    ssh_connect_timeout_seconds: int
    cooldown_seconds: int
    require_shared_state_ok: bool
    expect_anonymous_enabled: bool
    check_session_info: bool
    check_subscription_tiers: bool


@dataclass
class CheckResult:
    name: str
    ok: bool
    severity: str
    message: str
    context: dict[str, Any]


class CommandExecutor:
    def __init__(self, remote_host: str | None, ssh_connect_timeout_seconds: int) -> None:
        self.remote_host = remote_host.strip() if remote_host else None
        self.ssh_connect_timeout_seconds = ssh_connect_timeout_seconds

    def run(self, command: str, *, timeout_seconds: int, use_sudo: bool = False) -> subprocess.CompletedProcess[str]:
        if self.remote_host:
            remote_command = f"sudo {command}" if use_sudo else command
            args = [
                "ssh",
                "-o",
                "BatchMode=yes",
                "-o",
                f"ConnectTimeout={self.ssh_connect_timeout_seconds}",
                self.remote_host,
                remote_command,
            ]
            return subprocess.run(args, capture_output=True, text=True, timeout=timeout_seconds)

        shell_command = command
        if use_sudo and os.geteuid() != 0:
            shell_command = f"sudo {shell_command}"
        return subprocess.run(
            ["bash", "-lc", shell_command],
            capture_output=True,
            text=True,
            timeout=timeout_seconds,
        )


def fetch_text(url: str, timeout_seconds: int) -> tuple[int, str]:
    req = urllib_request.Request(
        url,
        headers={
            "User-Agent": "6529-public-alerts/1.0",
            "Accept": "application/json, text/html;q=0.9, */*;q=0.8",
        },
    )
    with urllib_request.urlopen(req, timeout=timeout_seconds) as resp:
        body = resp.read().decode("utf-8", "replace")
        return resp.status, body


def fetch_json(url: str, timeout_seconds: int) -> tuple[int, dict[str, Any], str]:
    status, body = fetch_text(url, timeout_seconds)
    try:
        parsed = json.loads(body)
    except json.JSONDecodeError as exc:
        raise RuntimeError(f"non-JSON response: {exc}") from exc
    if not isinstance(parsed, dict):
        raise RuntimeError("JSON response was not an object")
    return status, parsed, body


def load_config() -> Config:
    public_site_url = trim_trailing_slash(os.getenv("PUBLIC_SITE_URL", "https://6529vpn.io").strip())
    environment = (
        os.getenv("PUBLIC_ALERT_ENV")
        or os.getenv("OBS_ENV")
        or os.getenv("NODE_ENV")
        or "production"
    ).strip()
    service_name = os.getenv("PUBLIC_ALERT_SERVICE_NAME", "6529-public-stack").strip()
    webhook_url = os.getenv("ALERT_WEBHOOK_URL", "").strip() or None
    webhook_bearer_token = os.getenv("ALERT_WEBHOOK_BEARER_TOKEN", "").strip() or None
    telegram_bot_token = os.getenv("ALERT_TELEGRAM_BOT_TOKEN", "").strip() or None
    telegram_chat_id = os.getenv("ALERT_TELEGRAM_CHAT_ID", "").strip() or None
    telegram_message_thread_id = os.getenv("ALERT_TELEGRAM_MESSAGE_THREAD_ID", "").strip() or None

    return Config(
        public_site_url=public_site_url,
        remote_host=os.getenv("PUBLIC_ALERT_REMOTE_HOST", "").strip() or None,
        service_name=service_name,
        environment=environment,
        webhook_url=webhook_url,
        webhook_bearer_token=webhook_bearer_token,
        telegram_bot_token=telegram_bot_token,
        telegram_chat_id=telegram_chat_id,
        telegram_message_thread_id=telegram_message_thread_id,
        gateway_service_name=os.getenv("PUBLIC_ALERT_GATEWAY_SERVICE_NAME", "sovereign-gateway").strip(),
        zk_api_service_name=os.getenv("PUBLIC_ALERT_ZK_API_SERVICE_NAME", "sovereign-zk-api").strip(),
        wg_interface=os.getenv("PUBLIC_ALERT_WG_INTERFACE", "wg0").strip(),
        state_file=Path(
            os.getenv(
                "PUBLIC_ALERT_STATE_FILE",
                "/var/lib/sovereign-vpn/public-alert-state.json",
            ).strip()
        ),
        http_timeout_seconds=env_int("PUBLIC_ALERT_HTTP_TIMEOUT_SECONDS", 15),
        command_timeout_seconds=env_int("PUBLIC_ALERT_COMMAND_TIMEOUT_SECONDS", 15),
        ssh_connect_timeout_seconds=env_int("PUBLIC_ALERT_SSH_CONNECT_TIMEOUT_SECONDS", 10),
        cooldown_seconds=env_int("PUBLIC_ALERT_COOLDOWN_SECONDS", 900),
        require_shared_state_ok=env_bool("PUBLIC_ALERT_REQUIRE_SHARED_STATE_OK", False),
        expect_anonymous_enabled=env_bool("PUBLIC_ALERT_EXPECT_ANONYMOUS_ENABLED", True),
        check_session_info=env_bool("PUBLIC_ALERT_CHECK_SESSION_INFO", True),
        check_subscription_tiers=env_bool("PUBLIC_ALERT_CHECK_SUBSCRIPTION_TIERS", True),
    )


def load_state(path: Path) -> dict[str, Any]:
    if not path.exists():
        return {"version": 1, "checks": {}}
    try:
        raw = json.loads(path.read_text())
    except (OSError, json.JSONDecodeError):
        return {"version": 1, "checks": {}}
    if not isinstance(raw, dict):
        return {"version": 1, "checks": {}}
    checks = raw.get("checks")
    if not isinstance(checks, dict):
        raw["checks"] = {}
    return raw


def save_state(path: Path, state: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp_path = path.with_suffix(path.suffix + ".tmp")
    tmp_path.write_text(json.dumps(state, indent=2, sort_keys=True) + "\n")
    tmp_path.replace(path)


def configured_delivery_channels(config: Config) -> list[str]:
    channels: list[str] = []
    if config.webhook_url:
        channels.append("webhook")
    if config.telegram_bot_token and config.telegram_chat_id:
        channels.append("telegram")
    return channels


def post_webhook_alert(config: Config, payload: dict[str, Any]) -> None:
    if not config.webhook_url:
        return
    data = json.dumps(payload).encode("utf-8")
    headers = {
        "Content-Type": "application/json",
        "User-Agent": "6529-public-alerts/1.0",
    }
    if config.webhook_bearer_token:
        headers["Authorization"] = f"Bearer {config.webhook_bearer_token}"
    req = urllib_request.Request(config.webhook_url, data=data, headers=headers, method="POST")
    with urllib_request.urlopen(req, timeout=config.http_timeout_seconds) as resp:
        if resp.status < 200 or resp.status >= 300:
            raise RuntimeError(f"alert webhook returned status {resp.status}")


def format_telegram_alert(payload: dict[str, Any]) -> str:
    context = payload.get("context") if isinstance(payload.get("context"), dict) else {}
    context_json = json.dumps(context, sort_keys=True, separators=(", ", ": ")) if context else "{}"
    lines = [
        f"[{str(payload.get('severity', 'warning')).upper()}] {payload.get('service', '6529-public-stack')}",
        f"env: {payload.get('environment', 'production')}",
        f"type: {payload.get('alertType', 'public_stack_alert')}",
        f"message: {payload.get('message', '')}",
        f"time: {payload.get('timestamp', now_iso())}",
    ]
    if context and context_json != "{}":
        lines.append(f"context: {context_json}")
    return "\n".join(lines)


def post_telegram_alert(config: Config, payload: dict[str, Any]) -> None:
    if not config.telegram_bot_token or not config.telegram_chat_id:
        return
    url = f"https://api.telegram.org/bot{config.telegram_bot_token}/sendMessage"
    body: dict[str, Any] = {
        "chat_id": config.telegram_chat_id,
        "text": format_telegram_alert(payload),
        "disable_web_page_preview": True,
    }
    if config.telegram_message_thread_id:
        try:
            body["message_thread_id"] = int(config.telegram_message_thread_id, 10)
        except ValueError as exc:
            raise RuntimeError("ALERT_TELEGRAM_MESSAGE_THREAD_ID must be an integer") from exc
    data = json.dumps(body).encode("utf-8")
    headers = {
        "Content-Type": "application/json",
        "User-Agent": "6529-public-alerts/1.0",
    }
    req = urllib_request.Request(url, data=data, headers=headers, method="POST")
    with urllib_request.urlopen(req, timeout=config.http_timeout_seconds) as resp:
        response_body = resp.read().decode("utf-8", "replace")
        if resp.status < 200 or resp.status >= 300:
            raise RuntimeError(f"telegram returned status {resp.status}")
    try:
        parsed = json.loads(response_body)
    except json.JSONDecodeError as exc:
        raise RuntimeError("telegram returned non-JSON response") from exc
    if not isinstance(parsed, dict) or not parsed.get("ok"):
        raise RuntimeError(f"telegram sendMessage failed: {parsed}")


def post_alert(config: Config, payload: dict[str, Any]) -> list[str]:
    delivered: list[str] = []
    if config.webhook_url:
        post_webhook_alert(config, payload)
        delivered.append("webhook")
    if config.telegram_bot_token and config.telegram_chat_id:
        post_telegram_alert(config, payload)
        delivered.append("telegram")
    return delivered


def check_frontend(config: Config) -> CheckResult:
    url = config.public_site_url
    try:
        status, body = fetch_text(url, config.http_timeout_seconds)
    except Exception as exc:
        return CheckResult(
            name="frontend_reachable",
            ok=False,
            severity="critical",
            message=f"Public frontend unreachable: {exc}",
            context={"url": url},
        )

    if status != 200:
        return CheckResult(
            name="frontend_reachable",
            ok=False,
            severity="critical",
            message=f"Public frontend returned HTTP {status}",
            context={"url": url, "status": status},
        )

    if "/assets/" not in body:
        return CheckResult(
            name="frontend_reachable",
            ok=False,
            severity="warning",
            message="Public frontend HTML did not contain a compiled asset reference",
            context={"url": url, "status": status},
        )

    return CheckResult(
        name="frontend_reachable",
        ok=True,
        severity="warning",
        message="Public frontend reachable",
        context={"url": url, "status": status},
    )


def check_gateway_health(config: Config) -> tuple[CheckResult, dict[str, Any] | None]:
    url = f"{config.public_site_url}/health"
    try:
        status, body, _ = fetch_json(url, config.http_timeout_seconds)
    except Exception as exc:
        return (
            CheckResult(
                name="gateway_health",
                ok=False,
                severity="critical",
                message=f"Gateway health endpoint unavailable: {exc}",
                context={"url": url},
            ),
            None,
        )

    health_status = str(body.get("status", "unknown"))
    shared_state = body.get("shared_state") if isinstance(body.get("shared_state"), dict) else {}
    active_sessions = body.get("active_sessions")
    active_peers = body.get("active_peers")

    if status != 200:
        return (
            CheckResult(
                name="gateway_health",
                ok=False,
                severity="critical",
                message=f"Gateway health returned HTTP {status}",
                context={"url": url, "status": status, "body": body},
            ),
            body,
        )

    if health_status != "ok":
        return (
            CheckResult(
                name="gateway_health",
                ok=False,
                severity="critical",
                message=f"Gateway health status is {health_status}",
                context={"url": url, "body": body},
            ),
            body,
        )

    if active_sessions == -1:
        return (
            CheckResult(
                name="gateway_health",
                ok=False,
                severity="critical",
                message="Gateway could not read active session count",
                context={"url": url, "body": body},
            ),
            body,
        )

    if config.require_shared_state_ok and shared_state.get("enabled") and shared_state.get("status") != "ok":
        return (
            CheckResult(
                name="gateway_health",
                ok=False,
                severity="critical",
                message="Gateway shared state is degraded",
                context={"url": url, "shared_state": shared_state},
            ),
            body,
        )

    if isinstance(active_sessions, int) and isinstance(active_peers, int) and active_sessions > 0 and active_peers == 0:
        return (
            CheckResult(
                name="gateway_health",
                ok=False,
                severity="warning",
                message="Gateway reports active sessions but no active peers",
                context={"url": url, "active_sessions": active_sessions, "active_peers": active_peers},
            ),
            body,
        )

    return (
        CheckResult(
            name="gateway_health",
            ok=True,
            severity="warning",
            message="Gateway health endpoint is healthy",
            context={
                "url": url,
                "active_sessions": active_sessions,
                "active_peers": active_peers,
                "shared_state": shared_state,
            },
        ),
        body,
    )


def check_zk_api_health(config: Config) -> tuple[CheckResult, dict[str, Any] | None]:
    url = f"{config.public_site_url}/api/health"
    try:
        status, body, _ = fetch_json(url, config.http_timeout_seconds)
    except Exception as exc:
        return (
            CheckResult(
                name="zk_api_health",
                ok=False,
                severity="critical",
                message=f"ZK API health endpoint unavailable: {exc}",
                context={"url": url},
            ),
            None,
        )

    api_status = str(body.get("status", "unknown"))
    sync_rows = body.get("sync") if isinstance(body.get("sync"), list) else []
    sync_errors = [
        row.get("proofType") or row.get("proof_type") or "unknown"
        for row in sync_rows
        if isinstance(row, dict) and row.get("status") == "error"
    ]

    if status != 200:
        return (
            CheckResult(
                name="zk_api_health",
                ok=False,
                severity="critical",
                message=f"ZK API health returned HTTP {status}",
                context={"url": url, "status": status, "body": body},
            ),
            body,
        )

    if api_status == "error":
        return (
            CheckResult(
                name="zk_api_health",
                ok=False,
                severity="critical",
                message="ZK API health status is error",
                context={"url": url, "body": body},
            ),
            body,
        )

    if api_status == "degraded" or sync_errors:
        return (
            CheckResult(
                name="zk_api_health",
                ok=False,
                severity="warning",
                message="ZK API health is degraded" if api_status == "degraded" else "ZK API sync contains error rows",
                context={"url": url, "status": api_status, "sync_errors": sync_errors, "body": body},
            ),
            body,
        )

    return (
        CheckResult(
            name="zk_api_health",
            ok=True,
            severity="warning",
            message="ZK API health endpoint is healthy",
            context={"url": url, "status": api_status},
        ),
        body,
    )


def check_meta(config: Config) -> CheckResult:
    url = f"{config.public_site_url}/api/meta"
    try:
        status, body, _ = fetch_json(url, config.http_timeout_seconds)
    except Exception as exc:
        return CheckResult(
            name="zk_api_meta",
            ok=False,
            severity="critical",
            message=f"ZK API metadata endpoint unavailable: {exc}",
            context={"url": url},
        )

    recommended = (
        body.get("sdk", {}).get("recommendedClientConfig", {})
        if isinstance(body.get("sdk"), dict)
        else {}
    )
    configured_api_url = trim_trailing_slash(str(recommended.get("apiUrl", "")).strip())
    anonymous = body.get("anonymousVpn") if isinstance(body.get("anonymousVpn"), dict) else {}

    if status != 200:
        return CheckResult(
            name="zk_api_meta",
            ok=False,
            severity="critical",
            message=f"ZK API metadata returned HTTP {status}",
            context={"url": url, "status": status, "body": body},
        )

    if configured_api_url and configured_api_url != config.public_site_url:
        return CheckResult(
            name="zk_api_meta",
            ok=False,
            severity="critical",
            message="ZK API metadata points clients at the wrong public API URL",
            context={"url": url, "configured_api_url": configured_api_url, "expected": config.public_site_url},
        )

    if config.expect_anonymous_enabled:
        if not anonymous.get("enabled"):
            return CheckResult(
                name="zk_api_meta",
                ok=False,
                severity="critical",
                message="Anonymous VPN metadata is not enabled",
                context={"url": url, "anonymousVpn": anonymous},
            )
        issuer = anonymous.get("issuer") if isinstance(anonymous.get("issuer"), dict) else {}
        if not issuer.get("enabled"):
            return CheckResult(
                name="zk_api_meta",
                ok=False,
                severity="critical",
                message="Anonymous VPN issuer metadata is not enabled",
                context={"url": url, "anonymousVpn": anonymous},
            )

    return CheckResult(
        name="zk_api_meta",
        ok=True,
        severity="warning",
        message="ZK API metadata is healthy",
        context={"url": url, "anonymous_enabled": anonymous.get("enabled", False)},
    )


def check_session_info(config: Config) -> CheckResult:
    url = f"{config.public_site_url}/session/info"
    try:
        status, body, _ = fetch_json(url, config.http_timeout_seconds)
    except Exception as exc:
        return CheckResult(
            name="session_info",
            ok=False,
            severity="critical",
            message=f"Session info endpoint unavailable: {exc}",
            context={"url": url},
        )

    if status != 200:
        return CheckResult(
            name="session_info",
            ok=False,
            severity="critical",
            message=f"Session info returned HTTP {status}",
            context={"url": url, "status": status, "body": body},
        )

    if not body.get("contract") or not body.get("node_operator"):
        return CheckResult(
            name="session_info",
            ok=False,
            severity="warning",
            message="Session info is missing required purchase metadata",
            context={"url": url, "body": body},
        )

    return CheckResult(
        name="session_info",
        ok=True,
        severity="warning",
        message="Session info endpoint is healthy",
        context={"url": url, "contract": body.get("contract"), "node_operator": body.get("node_operator")},
    )


def check_subscription_tiers(config: Config) -> CheckResult:
    url = f"{config.public_site_url}/subscription/tiers"
    try:
        status, body, _ = fetch_json(url, config.http_timeout_seconds)
    except Exception as exc:
        return CheckResult(
            name="subscription_tiers",
            ok=False,
            severity="critical",
            message=f"Subscription tiers endpoint unavailable: {exc}",
            context={"url": url},
        )

    tiers = body.get("tiers") if isinstance(body.get("tiers"), list) else []
    active_tiers = [
        tier.get("id")
        for tier in tiers
        if isinstance(tier, dict) and bool(tier.get("active"))
    ]

    if status != 200:
        return CheckResult(
            name="subscription_tiers",
            ok=False,
            severity="critical",
            message=f"Subscription tiers returned HTTP {status}",
            context={"url": url, "status": status, "body": body},
        )

    if not active_tiers:
        return CheckResult(
            name="subscription_tiers",
            ok=False,
            severity="critical",
            message="Subscription tiers endpoint has no active purchase tiers",
            context={"url": url, "body": body},
        )

    return CheckResult(
        name="subscription_tiers",
        ok=True,
        severity="warning",
        message="Subscription tiers endpoint is healthy",
        context={"url": url, "active_tiers": active_tiers},
    )


def check_service(executor: CommandExecutor, service_name: str, config: Config, check_name: str) -> CheckResult:
    cmd = f"systemctl is-active {shlex.quote(service_name)}"
    try:
        completed = executor.run(cmd, timeout_seconds=config.command_timeout_seconds)
    except Exception as exc:
        return CheckResult(
            name=check_name,
            ok=False,
            severity="critical",
            message=f"Could not query systemd status for {service_name}: {exc}",
            context={"service": service_name},
        )

    status = completed.stdout.strip() or completed.stderr.strip() or "unknown"
    if completed.returncode != 0 or status != "active":
        return CheckResult(
            name=check_name,
            ok=False,
            severity="critical",
            message=f"Service {service_name} is not active ({status})",
            context={"service": service_name, "stdout": completed.stdout.strip(), "stderr": completed.stderr.strip()},
        )

    return CheckResult(
        name=check_name,
        ok=True,
        severity="warning",
        message=f"Service {service_name} is active",
        context={"service": service_name},
    )


def check_wireguard(executor: CommandExecutor, config: Config) -> CheckResult:
    cmd = f"wg show {shlex.quote(config.wg_interface)}"
    try:
        completed = executor.run(
            cmd,
            timeout_seconds=config.command_timeout_seconds,
            use_sudo=bool(executor.remote_host),
        )
    except Exception as exc:
        return CheckResult(
            name="wireguard_interface",
            ok=False,
            severity="critical",
            message=f"Could not query WireGuard interface {config.wg_interface}: {exc}",
            context={"interface": config.wg_interface},
        )

    if completed.returncode != 0:
        stderr = completed.stderr.strip()
        stdout = completed.stdout.strip()
        return CheckResult(
            name="wireguard_interface",
            ok=False,
            severity="critical",
            message=f"WireGuard interface {config.wg_interface} is not healthy",
            context={"interface": config.wg_interface, "stdout": stdout, "stderr": stderr},
        )

    if f"interface: {config.wg_interface}" not in completed.stdout:
        return CheckResult(
            name="wireguard_interface",
            ok=False,
            severity="critical",
            message=f"WireGuard output did not include interface {config.wg_interface}",
            context={"interface": config.wg_interface, "stdout": completed.stdout.strip()},
        )

    peer_count = sum(1 for line in completed.stdout.splitlines() if line.startswith("peer: "))
    return CheckResult(
        name="wireguard_interface",
        ok=True,
        severity="warning",
        message=f"WireGuard interface {config.wg_interface} is healthy",
        context={"interface": config.wg_interface, "peer_count": peer_count},
    )


def evaluate_checks(config: Config) -> list[CheckResult]:
    executor = CommandExecutor(config.remote_host, config.ssh_connect_timeout_seconds)
    results: list[CheckResult] = []

    results.append(check_frontend(config))

    gateway_check, _ = check_gateway_health(config)
    results.append(gateway_check)

    api_check, _ = check_zk_api_health(config)
    results.append(api_check)

    results.append(check_meta(config))

    if config.check_session_info:
        results.append(check_session_info(config))
    if config.check_subscription_tiers:
        results.append(check_subscription_tiers(config))

    results.append(
        check_service(
            executor,
            config.gateway_service_name,
            config,
            "gateway_service",
        )
    )
    results.append(
        check_service(
            executor,
            config.zk_api_service_name,
            config,
            "zk_api_service",
        )
    )
    results.append(check_wireguard(executor, config))

    return results


def build_alert_payload(
    *,
    config: Config,
    alert_type: str,
    severity: str,
    message: str,
    context: dict[str, Any],
) -> dict[str, Any]:
    return {
        "timestamp": now_iso(),
        "service": config.service_name,
        "environment": config.environment,
        "alertType": alert_type,
        "severity": severity,
        "message": message,
        "context": context,
    }


def apply_notifications(
    *,
    config: Config,
    state: dict[str, Any],
    results: list[CheckResult],
    dry_run: bool,
) -> tuple[list[dict[str, Any]], list[str], dict[str, Any]]:
    notifications: list[dict[str, Any]] = []
    delivery_errors: list[str] = []
    checks_state = state.setdefault("checks", {})
    now_epoch = int(time.time())

    for result in results:
        previous = checks_state.get(result.name, {})
        previous_status = previous.get("status", "ok")
        last_alert_at = previous.get("last_alert_at")
        last_message = previous.get("last_message")

        if result.ok:
            if previous_status == "failing" and previous.get("last_alert_kind") == "failure":
                payload = build_alert_payload(
                    config=config,
                    alert_type=f"public_stack_{result.name}_recovered",
                    severity="warning",
                    message=f"{result.name} recovered",
                    context=result.context,
                )
                notifications.append(payload)
                if not dry_run:
                    try:
                        post_alert(config, payload)
                    except Exception as exc:
                        delivery_errors.append(
                            f"recovery alert for {result.name} failed: {exc}"
                        )
            checks_state[result.name] = {
                "status": "ok",
                "last_checked_at": now_epoch,
                "last_message": result.message,
                "last_severity": result.severity,
                "last_alert_at": None,
                "last_alert_kind": None,
                "first_failed_at": None,
            }
            continue

        should_alert = False
        if previous_status != "failing":
            should_alert = True
        elif last_alert_at is None:
            should_alert = True
        elif last_message != result.message:
            should_alert = True
        elif now_epoch - int(last_alert_at) >= config.cooldown_seconds:
            should_alert = True

        if should_alert:
            payload = build_alert_payload(
                config=config,
                alert_type=f"public_stack_{result.name}",
                severity=result.severity,
                message=result.message,
                context=result.context,
            )
            notifications.append(payload)
            if not dry_run:
                try:
                    post_alert(config, payload)
                except Exception as exc:
                    delivery_errors.append(
                        f"alert for {result.name} failed: {exc}"
                    )

        checks_state[result.name] = {
            "status": "failing",
            "last_checked_at": now_epoch,
            "last_message": result.message,
            "last_severity": result.severity,
            "last_alert_at": now_epoch if should_alert else last_alert_at,
            "last_alert_kind": "failure" if should_alert else previous.get("last_alert_kind"),
            "first_failed_at": previous.get("first_failed_at") or now_epoch,
        }

    state["version"] = 1
    state["updated_at"] = now_epoch
    return notifications, delivery_errors, state


def print_human_summary(results: list[CheckResult], delivery_channels: list[str], remote_host: str | None) -> None:
    channels_label = ", ".join(delivery_channels) if delivery_channels else "none"
    print(f"Alert delivery channels: {channels_label}")
    print(f"Remote host checks: {remote_host if remote_host else 'local'}")
    for result in results:
        label = "OK" if result.ok else result.severity.upper()
        print(f"{label:<8} {result.name}: {result.message}")


def main() -> int:
    parser = argparse.ArgumentParser(description="Check the public 6529 VPN stack and emit webhook alerts.")
    parser.add_argument("--dry-run", action="store_true", help="Evaluate checks without sending alerts or writing state.")
    parser.add_argument("--json", action="store_true", help="Print a JSON summary instead of a human-readable summary.")
    args = parser.parse_args()

    config = load_config()
    state = load_state(config.state_file)
    results = evaluate_checks(config)
    notifications, delivery_errors, updated_state = apply_notifications(
        config=config,
        state=state,
        results=results,
        dry_run=args.dry_run,
    )

    summary = {
        "timestamp": now_iso(),
        "service": config.service_name,
        "environment": config.environment,
        "publicSiteUrl": config.public_site_url,
        "webhookConfigured": bool(config.webhook_url),
        "telegramConfigured": bool(config.telegram_bot_token and config.telegram_chat_id),
        "deliveryChannels": configured_delivery_channels(config),
        "remoteHost": config.remote_host,
        "dryRun": args.dry_run,
        "notifications": notifications,
        "deliveryErrors": delivery_errors,
        "checks": [
            {
                "name": result.name,
                "ok": result.ok,
                "severity": result.severity,
                "message": result.message,
                "context": result.context,
            }
            for result in results
        ],
    }

    if args.json:
        print(json.dumps(summary, indent=2, sort_keys=True))
    else:
        print_human_summary(results, configured_delivery_channels(config), config.remote_host)
        if notifications:
            print()
            print("Notifications:")
            for payload in notifications:
                print(f"- {payload['alertType']}: {payload['message']}")
        if delivery_errors:
            print()
            print("Delivery errors:")
            for line in delivery_errors:
                print(f"- {line}")

    if not args.dry_run:
        save_state(config.state_file, updated_state)

    return 0 if all(result.ok for result in results) else 1


if __name__ == "__main__":
    try:
        sys.exit(main())
    except urllib_error.HTTPError as exc:
        print(f"fatal HTTP error: {exc}", file=sys.stderr)
        sys.exit(2)
    except Exception as exc:
        print(f"fatal error: {exc}", file=sys.stderr)
        sys.exit(2)
