import { useState } from 'react';
import GatewayStatus from './GatewayStatus';
import VPNConnect from './VPNConnect';
import NodeSelector from './NodeSelector';
import SessionDashboard from './SessionDashboard';
import OperatorEnrollment from './OperatorEnrollment';
import { useSession } from './useSession';

export default function App() {
  const [gatewayUrl, setGatewayUrl] = useState('');
  const [view, setView] = useState('connect');
  const { session, saveSession, clearSession } = useSession();

  const handleRenew = (newExpiresAt) => {
    saveSession({ ...session, expiresAt: new Date(newExpiresAt * 1000).toISOString() });
  };

  const showOperator = view === 'operator';
  const showSession = session && !showOperator;

  return (
    <div className="container">
      <div className="hero">
        <h1><span>6529</span> VPN</h1>
        <p>
          An NFT-gated VPN for the 6529 community. Hold a Memes card, get a VPN.
          No accounts. No emails. No KYC.
        </p>
        <GatewayStatus gatewayUrl={gatewayUrl} />
      </div>

      <div className="view-tabs">
        <button
          className={view === 'connect' ? 'active' : ''}
          onClick={() => setView('connect')}
        >
          Connect
        </button>
        <button
          className={showOperator ? 'active' : ''}
          onClick={() => setView('operator')}
        >
          Run a Node
        </button>
      </div>

      {showOperator ? (
        <OperatorEnrollment />
      ) : showSession ? (
        <SessionDashboard
          session={session}
          onDisconnect={clearSession}
          onReconnect={clearSession}
          onRenew={handleRenew}
        />
      ) : (
        <>
          <NodeSelector onSelect={setGatewayUrl} />

          <VPNConnect gatewayUrl={gatewayUrl} onSessionCreated={saveSession} />

          <section>
            <h2>How It Works</h2>
            <div className="steps">
              <div className="step">
                <div className="step-num">1</div>
                <div className="step-text">
                  <strong>Hold a Memes card</strong>
                  <span>
                    Any card from{' '}
                    <a href="https://6529.io/the-memes" target="_blank" rel="noreferrer">The Memes by 6529</a>{' '}
                    in your wallet gets you access.
                  </span>
                </div>
              </div>
              <div className="step">
                <div className="step-num">2</div>
                <div className="step-text">
                  <strong>Connect your wallet above</strong>
                  <span>
                    Click "Connect Wallet", sign a message, and the gateway checks your Memes ownership on-chain.
                  </span>
                </div>
              </div>
              <div className="step">
                <div className="step-num">3</div>
                <div className="step-text">
                  <strong>Import into WireGuard</strong>
                  <span>
                    Download the config file and import it into the{' '}
                    <a href="https://www.wireguard.com/install/" target="_blank" rel="noreferrer">WireGuard app</a>{' '}
                    on any device.
                  </span>
                </div>
              </div>
            </div>
          </section>

          <section>
            <h2>Access</h2>
            <div className="steps">
              <div className="step">
                <div className="step-num" style={{ background: '#2196f3' }}>P</div>
                <div className="step-text">
                  <strong>Paid Access</strong>
                  <span>Hold a supported Memes card. Access is currently paid while pricing and incentives are tested.</span>
                </div>
              </div>
              <div className="step">
                <div className="step-num" style={{ background: 'var(--success)' }}>?</div>
                <div className="step-text">
                  <strong>Future Sponsored Access</strong>
                  <span>The project's card may unlock sponsored access later, pending community feedback.</span>
                </div>
              </div>
            </div>
          </section>

          <section>
            <h2>Requirements</h2>
            <ul className="req-list">
              <li><strong>A Memes card</strong> in your Ethereum wallet (any card from The Memes by 6529)</li>
              <li>
                <strong>WireGuard app</strong> installed on your device (
                <a href="https://www.wireguard.com/install/" target="_blank" rel="noreferrer">wireguard.com/install</a>)
              </li>
              <li>
                <strong>A wallet</strong> — MetaMask, Rainbow, Coinbase Wallet, WalletConnect, and more
              </li>
            </ul>
          </section>

          <section>
            <h2>CLI Client</h2>
            <p style={{ color: 'var(--muted)', marginBottom: 16 }}>
              Prefer the command line? Build <code>svpn</code> from source while packaged releases are being prepared:
            </p>
            <a
              className="dl-btn"
              href="https://github.com/maybehotcarl/sovereign-vpn/tree/main/client"
              target="_blank"
              rel="noreferrer"
            >
              <div><div className="os">CLI Source</div><div className="arch">Go client for WireGuard sessions</div></div>
            </a>
            <div className="code-block" style={{ marginTop: 16 }}>
              <span className="cmd">cd client && go build -o ../bin/svpn ./cmd/svpn</span>
            </div>
          </section>
        </>
      )}

      <footer>
        <div className="footer-links">
          <a href="https://github.com/maybehotcarl/sovereign-vpn">GitHub</a>
          <a href="https://6529.io/the-memes">The Memes</a>
          <a href="https://6529.io">6529</a>
          <a href="/health" rel="nofollow">API Status</a>
        </div>
        <p>Built for the 6529 community</p>
      </footer>
    </div>
  );
}
