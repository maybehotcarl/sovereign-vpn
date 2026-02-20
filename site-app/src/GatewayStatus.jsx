import { useState, useEffect } from 'react';

export default function GatewayStatus() {
  const [status, setStatus] = useState({ online: null, peers: 0 });

  useEffect(() => {
    fetch('/health')
      .then(r => r.json())
      .then(data => {
        setStatus({
          online: data.status === 'ok',
          peers: data.active_peers || 0,
        });
      })
      .catch(() => setStatus({ online: false, peers: 0 }));
  }, []);

  if (status.online === null) {
    return (
      <div className="status">
        <div className="status-dot" style={{ background: 'var(--muted)' }} />
        <span>Checking gateway...</span>
      </div>
    );
  }

  return (
    <div className="status">
      <div className={`status-dot${status.online ? '' : ' offline'}`} />
      <span>
        {status.online
          ? `Gateway online \u2022 ${status.peers} active peer${status.peers !== 1 ? 's' : ''}`
          : 'Gateway offline'}
      </span>
    </div>
  );
}
