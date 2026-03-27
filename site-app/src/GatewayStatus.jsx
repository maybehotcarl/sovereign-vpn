import { useEffect, useState } from 'react';

const HISTORY_LIMIT = 24;
const POLL_INTERVAL_MS = 10000;
const GRAPH_WIDTH = 320;
const GRAPH_HEIGHT = 96;
const GRAPH_PADDING_X = 10;
const GRAPH_PADDING_Y = 10;

function formatLatency(latencyMs) {
  if (latencyMs == null) return 'Offline';
  if (latencyMs < 1000) return `${latencyMs}ms`;
  return `${(latencyMs / 1000).toFixed(1)}s`;
}

function formatStatusTime(value) {
  if (!value) return 'Waiting for first response';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return 'Waiting for first response';
  return date.toLocaleTimeString([], {
    hour: 'numeric',
    minute: '2-digit',
    second: '2-digit',
  });
}

function toneForStatus(status) {
  if (status.online === null) return 'checking';
  if (!status.online) return 'offline';
  if (status.sharedState === 'error') return 'degraded';
  return 'online';
}

function titleForTone(tone) {
  if (tone === 'online') return 'Healthy';
  if (tone === 'degraded') return 'Degraded';
  if (tone === 'offline') return 'Offline';
  return 'Checking';
}

function sharedStateLabel(status) {
  if (status.online === null) return 'Pending';
  if (!status.online) return 'Unavailable';
  if (status.sharedState === 'error') return 'Degraded';
  if (status.sharedState === 'ok') return 'Healthy';
  return 'Standalone';
}

function sampleBarHeight(latencyMs, maxLatency) {
  if (latencyMs == null) return 6;
  return Math.max(8, Math.round((latencyMs / maxLatency) * (GRAPH_HEIGHT - GRAPH_PADDING_Y * 2)));
}

function sampleX(index, count) {
  if (count <= 1) {
    return GRAPH_WIDTH / 2;
  }
  const usableWidth = GRAPH_WIDTH - GRAPH_PADDING_X * 2;
  return GRAPH_PADDING_X + (usableWidth * index) / (count - 1);
}

function buildPeerPath(history, maxPeers) {
  if (history.length === 0) return '';
  return history
    .map((sample, index) => {
      const x = sampleX(index, history.length);
      const y =
        GRAPH_HEIGHT -
        GRAPH_PADDING_Y -
        (sample.peers / maxPeers) * (GRAPH_HEIGHT - GRAPH_PADDING_Y * 2);
      return `${x},${y}`;
    })
    .join(' ');
}

function statusTargetLabel(gatewayUrl, gatewayInstanceId) {
  if (gatewayInstanceId) return gatewayInstanceId;
  if (!gatewayUrl) return 'default gateway';
  try {
    return new URL(gatewayUrl).host;
  } catch {
    return gatewayUrl.replace(/^https?:\/\//i, '');
  }
}

export default function GatewayStatus({ gatewayUrl = '' }) {
  const [status, setStatus] = useState({
    online: null,
    peers: 0,
    latencyMs: null,
    sharedState: 'unknown',
    checkedAt: null,
    gatewayInstanceId: '',
    history: [],
  });

  useEffect(() => {
    let active = true;

    const poll = async () => {
      const startedAt = performance.now();
      try {
        const response = await fetch(`${gatewayUrl}/health`, { cache: 'no-store' });
        if (!response.ok) {
          throw new Error(`Health endpoint returned ${response.status}`);
        }
        const data = await response.json();
        if (!active) return;

        const nextSample = {
          online: data.status === 'ok',
          peers: Number.isFinite(data.active_peers) ? data.active_peers : 0,
          latencyMs: Math.round(performance.now() - startedAt),
          sharedState: data.shared_state?.status || 'standalone',
          checkedAt: data.time || new Date().toISOString(),
          gatewayInstanceId: data.gateway_instance_id || '',
        };

        setStatus((previous) => ({
          ...nextSample,
          history: [...previous.history, nextSample].slice(-HISTORY_LIMIT),
        }));
      } catch {
        if (!active) return;

        const nextSample = {
          online: false,
          peers: 0,
          latencyMs: null,
          sharedState: 'error',
          checkedAt: new Date().toISOString(),
          gatewayInstanceId: '',
        };

        setStatus((previous) => ({
          ...nextSample,
          history: [...previous.history, nextSample].slice(-HISTORY_LIMIT),
        }));
      }
    };

    poll();
    const interval = window.setInterval(poll, POLL_INTERVAL_MS);

    return () => {
      active = false;
      window.clearInterval(interval);
    };
  }, [gatewayUrl]);

  const tone = toneForStatus(status);
  const history = status.history;
  const maxLatency = Math.max(
    250,
    ...history.map((sample) => sample.latencyMs ?? 0)
  );
  const maxPeers = Math.max(1, ...history.map((sample) => sample.peers));
  const peerPath = buildPeerPath(history, maxPeers);
  const target = statusTargetLabel(gatewayUrl, status.gatewayInstanceId);

  return (
    <div className={`status-panel status-panel-${tone}`}>
      <div className="status-panel-header">
        <div>
          <div className="status-kicker">API Status</div>
          <div className="status-title-row">
            <div className={`status-dot${tone === 'offline' ? ' offline' : ''}`} />
            <div className="status-title">{titleForTone(tone)}</div>
          </div>
          <div className="status-subtitle">
            Monitoring {target} • last check {formatStatusTime(status.checkedAt)}
          </div>
        </div>
        <div className="status-summary">
          {history.length > 0 ? `${history.length} samples` : 'Starting stream'}
        </div>
      </div>

      <div className="status-metrics">
        <div className="status-metric-card">
          <div className="status-metric-label">Response Time</div>
          <div className="status-metric-value">{formatLatency(status.latencyMs)}</div>
        </div>
        <div className="status-metric-card">
          <div className="status-metric-label">Active Peers</div>
          <div className="status-metric-value">{status.online === null ? '...' : status.peers}</div>
        </div>
        <div className="status-metric-card">
          <div className="status-metric-label">Shared State</div>
          <div className="status-metric-value">{sharedStateLabel(status)}</div>
        </div>
      </div>

      <div className="status-graph-card">
        <div className="status-graph-header">
          <span>Last {HISTORY_LIMIT} health checks</span>
          <span>Latency bars + peer trend</span>
        </div>
        <svg
          className="status-graph"
          viewBox={`0 0 ${GRAPH_WIDTH} ${GRAPH_HEIGHT}`}
          role="img"
          aria-label="Gateway health graph"
        >
          {[0.25, 0.5, 0.75].map((ratio) => {
            const y = GRAPH_HEIGHT - GRAPH_PADDING_Y - ratio * (GRAPH_HEIGHT - GRAPH_PADDING_Y * 2);
            return (
              <line
                key={ratio}
                x1={GRAPH_PADDING_X}
                x2={GRAPH_WIDTH - GRAPH_PADDING_X}
                y1={y}
                y2={y}
                className="status-grid-line"
              />
            );
          })}

          {history.map((sample, index) => {
            const barWidth = history.length > 0
              ? Math.max(6, (GRAPH_WIDTH - GRAPH_PADDING_X * 2) / Math.max(history.length, 1) - 4)
              : 6;
            const x = sampleX(index, history.length) - barWidth / 2;
            const barHeight = sampleBarHeight(sample.latencyMs, maxLatency);
            const y = GRAPH_HEIGHT - GRAPH_PADDING_Y - barHeight;
            const toneClass = !sample.online
              ? 'offline'
              : sample.sharedState === 'error'
                ? 'degraded'
                : 'online';

            return (
              <rect
                key={`${sample.checkedAt}-${index}`}
                x={x}
                y={y}
                width={barWidth}
                height={barHeight}
                rx="3"
                className={`status-bar status-bar-${toneClass}`}
              />
            );
          })}

          {peerPath && (
            <>
              <polyline
                points={peerPath}
                fill="none"
                className="status-peer-line"
              />
              {history.length > 0 && (
                <circle
                  cx={sampleX(history.length - 1, history.length)}
                  cy={
                    GRAPH_HEIGHT -
                    GRAPH_PADDING_Y -
                    (history[history.length - 1].peers / maxPeers) * (GRAPH_HEIGHT - GRAPH_PADDING_Y * 2)
                  }
                  r="4"
                  className="status-peer-dot"
                />
              )}
            </>
          )}
        </svg>
        <div className="status-legend">
          <span className="status-legend-item">
            <span className="status-legend-bar-swatch" />
            Response time
          </span>
          <span className="status-legend-item">
            <span className="status-legend-line-swatch" />
            Active peers
          </span>
        </div>
      </div>
    </div>
  );
}
