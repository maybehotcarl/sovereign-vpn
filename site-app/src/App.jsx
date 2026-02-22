import { useState } from 'react';
import GatewayStatus from './GatewayStatus';
import VPNConnect from './VPNConnect';
import NodeSelector from './NodeSelector';
import SessionDashboard from './SessionDashboard';
import { useSession } from './useSession';

export default function App() {
  const [gatewayUrl, setGatewayUrl] = useState('');
  const { session, saveSession, clearSession } = useSession();

  // Active or expired session — show dashboard
  if (session) {
    return (
      <div className="container">
        <div className="hero">
          <h1><span>6529</span> VPN</h1>
          <p>
            An NFT-gated VPN for the 6529 community. Hold a Memes card, get a VPN.
            No accounts. No emails. No KYC.
          </p>
          <GatewayStatus gatewayUrl={session.gatewayUrl} />
        </div>

        <SessionDashboard
          session={session}
          onDisconnect={clearSession}
          onReconnect={clearSession}
        />

        <footer>
          <div className="footer-links">
            <a href="https://github.com/maybehotcarl/sovereign-vpn">GitHub</a>
            <a href="https://6529.io/the-memes">The Memes</a>
            <a href="https://6529.io">6529</a>
            <a href="/health">API Status</a>
          </div>
          <p>Built for the 6529 community</p>
        </footer>
      </div>
    );
  }

  // No session — show connect flow + info sections
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
        <h2>Access Tiers</h2>
        <div className="steps">
          <div className="step">
            <div className="step-num" style={{ background: 'var(--success)' }}>F</div>
            <div className="step-text">
              <strong>Free Tier</strong>
              <span>Hold the project's Memes card (THIS card) and use the VPN for free.</span>
            </div>
          </div>
          <div className="step">
            <div className="step-num" style={{ background: '#2196f3' }}>P</div>
            <div className="step-text">
              <strong>Paid Tier</strong>
              <span>Hold any other Memes card. Access for a small fee that supports node operators.</span>
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
          Prefer the command line? Download <code>svpn</code> for your platform:
        </p>
        <div className="downloads">
          <a href="/downloads/svpn-darwin-arm64" className="dl-btn">
            <div><div className="os">macOS</div><div className="arch">Apple Silicon (M1/M2/M3)</div></div>
          </a>
          <a href="/downloads/svpn-darwin-amd64" className="dl-btn">
            <div><div className="os">macOS</div><div className="arch">Intel</div></div>
          </a>
          <a href="/downloads/svpn-linux-amd64" className="dl-btn">
            <div><div className="os">Linux</div><div className="arch">x86_64</div></div>
          </a>
          <a href="/downloads/svpn-windows-amd64.exe" className="dl-btn">
            <div><div className="os">Windows</div><div className="arch">x86_64</div></div>
          </a>
        </div>
        <div className="code-block" style={{ marginTop: 16 }}>
          <span className="cmd">./svpn connect --gateway https://6529vpn.io --key wallet.key</span>
        </div>
      </section>

      <footer>
        <div className="footer-links">
          <a href="https://github.com/maybehotcarl/sovereign-vpn">GitHub</a>
          <a href="https://6529.io/the-memes">The Memes</a>
          <a href="https://6529.io">6529</a>
          <a href="/health">API Status</a>
        </div>
        <p>Built for the 6529 community</p>
      </footer>
    </div>
  );
}
