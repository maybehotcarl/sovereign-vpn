import { useState, useEffect } from 'react';

const MAIN_GATEWAY = {
  operator: 'Main Gateway',
  endpoint: '6529vpn.io:51820',
  region: 'us-east',
  rep: 0,
  gatewayUrl: '',
  isMain: true,
};

export default function NodeSelector({ onSelect }) {
  const [nodes, setNodes] = useState([]);
  const [selected, setSelected] = useState(MAIN_GATEWAY);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetch('/nodes')
      .then(r => r.ok ? r.json() : null)
      .then(data => {
        if (data && data.nodes) {
          const enriched = data.nodes.map(n => {
            const hostname = n.endpoint.split(':')[0];
            return { ...n, gatewayUrl: `https://${hostname}` };
          });
          setNodes(enriched);
        }
      })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  const handleSelect = (node) => {
    setSelected(node);
    onSelect(node.isMain ? '' : node.gatewayUrl);
  };

  // Group nodes by region
  const grouped = {};
  for (const n of nodes) {
    const r = n.region || 'unknown';
    if (!grouped[r]) grouped[r] = [];
    grouped[r].push(n);
  }

  return (
    <div className="node-selector">
      <h2>Select a Node</h2>
      <p style={{ color: 'var(--muted)', marginBottom: 16 }}>
        Choose which VPN node to connect through.
      </p>

      <div className="node-grid">
        {/* Main gateway is always first */}
        <button
          className={`node-card${selected === MAIN_GATEWAY ? ' selected' : ''}`}
          onClick={() => handleSelect(MAIN_GATEWAY)}
        >
          <div className="node-region">Main</div>
          <div className="node-name">6529vpn.io</div>
          <div className="node-meta">Official gateway</div>
        </button>

        {loading && (
          <div className="node-card" style={{ opacity: 0.5, cursor: 'default' }}>
            <div className="node-meta">Loading nodes...</div>
          </div>
        )}

        {Object.entries(grouped).map(([region, regionNodes]) =>
          regionNodes.map((n) => (
            <button
              key={n.endpoint}
              className={`node-card${selected === n ? ' selected' : ''}`}
              onClick={() => handleSelect(n)}
            >
              <div className="node-region">{region}</div>
              <div className="node-name">{n.endpoint.split(':')[0]}</div>
              <div className="node-meta">
                {n.operator.slice(0, 6)}...{n.operator.slice(-4)} &middot; rep {n.rep}
              </div>
            </button>
          ))
        )}
      </div>
    </div>
  );
}
