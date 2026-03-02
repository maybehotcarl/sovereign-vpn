import { useState, useCallback } from 'react';

const STORAGE_KEY = 'svpn_session';

function clearPersistedSession() {
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch {
    // no-op
  }
}

export function useSession() {
  // Keep VPN session data in memory only.
  // Legacy persisted session blobs are purged immediately for safety.
  const [session, setSession] = useState(() => {
    clearPersistedSession();
    return null;
  });

  const saveSession = useCallback((data) => {
    clearPersistedSession();
    setSession(data);
  }, []);

  const clearSession = useCallback(() => {
    clearPersistedSession();
    setSession(null);
  }, []);

  return { session, saveSession, clearSession };
}
