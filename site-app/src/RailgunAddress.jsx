import { useState, useEffect, useCallback } from 'react';
import { useAccount, useReadContract, useWriteContract, useWaitForTransactionReceipt } from 'wagmi';
import { NODE_REGISTRY_ADDRESS, NODE_REGISTRY_ABI } from './contracts';

export default function RailgunAddress() {
  const { address, isConnected } = useAccount();
  const [input, setInput] = useState('');
  const [txHash, setTxHash] = useState(null);
  const [error, setError] = useState('');
  const [phase, setPhase] = useState('idle'); // idle|sending|confirming|done|error

  const { writeContractAsync } = useWriteContract();
  const { isSuccess: txConfirmed, isError: txFailed } = useWaitForTransactionReceipt({ hash: txHash });

  // Read current RAILGUN address from contract
  const { data: currentAddress, refetch } = useReadContract({
    address: NODE_REGISTRY_ADDRESS,
    abi: NODE_REGISTRY_ABI,
    functionName: 'getRailgunAddress',
    args: [address],
    enabled: isConnected && !!address && !!NODE_REGISTRY_ADDRESS,
  });

  // Read registration status
  const { data: isRegistered } = useReadContract({
    address: NODE_REGISTRY_ADDRESS,
    abi: NODE_REGISTRY_ABI,
    functionName: 'isRegistered',
    args: [address],
    enabled: isConnected && !!address && !!NODE_REGISTRY_ADDRESS,
  });

  // Watch TX confirmation
  useEffect(() => {
    if (txConfirmed && phase === 'confirming') {
      setPhase('done');
      refetch();
      setTimeout(() => setPhase('idle'), 3000);
    }
    if (txFailed && phase === 'confirming') {
      setError('Transaction failed on-chain');
      setPhase('error');
    }
  }, [txConfirmed, txFailed, phase, refetch]);

  const handleSubmit = useCallback(async (e) => {
    e.preventDefault();
    setError('');

    // Client-side validation
    if (!input.startsWith('0zk')) {
      setError('RAILGUN address must start with "0zk"');
      return;
    }
    if (input.length < 10) {
      setError('RAILGUN address too short');
      return;
    }

    setPhase('sending');
    try {
      const hash = await writeContractAsync({
        address: NODE_REGISTRY_ADDRESS,
        abi: NODE_REGISTRY_ABI,
        functionName: 'setRailgunAddress',
        args: [input],
      });
      setTxHash(hash);
      setPhase('confirming');
    } catch (err) {
      setError(err.shortMessage || err.message || 'Transaction failed');
      setPhase('error');
    }
  }, [input, writeContractAsync]);

  if (!isConnected) return null;
  if (!NODE_REGISTRY_ADDRESS) return null;
  if (!isRegistered) return null;

  return (
    <div className="railgun-address-panel">
      <h3>RAILGUN Private Payout Address</h3>
      <p style={{ color: 'var(--muted)', fontSize: '0.85rem', marginBottom: 12 }}>
        Register your RAILGUN 0zk address to receive private payouts.
        Operator earnings will be shielded and sent to this address weekly.
      </p>

      {currentAddress && (
        <div style={{ marginBottom: 12 }}>
          <div className="stat-label">Current Address</div>
          <div className="stat-value mono" style={{ fontSize: '0.8rem', wordBreak: 'break-all' }}>
            {currentAddress}
          </div>
        </div>
      )}

      <form onSubmit={handleSubmit}>
        <div style={{ display: 'flex', gap: 8 }}>
          <input
            type="text"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="0zk..."
            className="railgun-input"
            disabled={phase === 'sending' || phase === 'confirming'}
            style={{ flex: 1, padding: '10px 12px', fontFamily: 'monospace', fontSize: '0.85rem' }}
          />
          <button
            type="submit"
            className="btn-primary"
            disabled={!input || phase === 'sending' || phase === 'confirming'}
            style={{ padding: '10px 20px', fontSize: '0.85rem', whiteSpace: 'nowrap' }}
          >
            {phase === 'sending' ? 'Confirm in wallet...' :
             phase === 'confirming' ? 'Confirming...' :
             currentAddress ? 'Update' : 'Register'}
          </button>
        </div>
      </form>

      {phase === 'done' && (
        <p style={{ color: 'var(--success)', marginTop: 8, fontSize: '0.85rem' }}>
          RAILGUN address updated!
        </p>
      )}

      {error && (
        <p style={{ color: 'var(--error)', marginTop: 8, fontSize: '0.85rem' }}>
          {error}
          {phase === 'error' && (
            <button
              className="btn-secondary"
              onClick={() => { setPhase('idle'); setError(''); }}
              style={{ marginLeft: 8, padding: '4px 12px', fontSize: '0.8rem' }}
            >
              Dismiss
            </button>
          )}
        </p>
      )}
    </div>
  );
}
