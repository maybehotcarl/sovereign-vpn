import { useState, useCallback } from 'react';

const STORAGE_KEY = 'svpn_session';

function loadSession() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    return JSON.parse(raw);
  } catch {
    return null;
  }
}

export function useSession() {
  const [session, setSession] = useState(loadSession);

  const saveSession = useCallback((data) => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(data));
    setSession(data);
  }, []);

  const clearSession = useCallback(() => {
    localStorage.removeItem(STORAGE_KEY);
    setSession(null);
  }, []);

  return { session, saveSession, clearSession };
}
