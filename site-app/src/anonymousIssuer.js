function normalizeBaseUrl(value) {
  return value.replace(/\/$/, '');
}

function resolveEndpoint(baseUrl, endpoint) {
  if (!endpoint) {
    return normalizeBaseUrl(baseUrl);
  }
  if (/^https?:\/\//i.test(endpoint)) {
    return endpoint;
  }
  const normalizedBase = normalizeBaseUrl(baseUrl);
  return `${normalizedBase}${endpoint.startsWith('/') ? endpoint : `/${endpoint}`}`;
}

async function parseResponse(response) {
  try {
    return await response.json();
  } catch {
    return null;
  }
}

function responseErrorMessage(payload, fallback) {
  if (payload && typeof payload === 'object') {
    if (typeof payload.error === 'string' && payload.error.trim()) {
      return payload.error;
    }
    if (typeof payload.message === 'string' && payload.message.trim()) {
      return payload.message;
    }
  }
  return fallback;
}

export class AnonymousIssuerRequestError extends Error {
  constructor(message, status, code) {
    super(message);
    this.name = 'AnonymousIssuerRequestError';
    this.status = status;
    this.code = code;
  }
}

async function postJSON(url, body, init = {}, fallbackError = 'Request failed') {
  const response = await fetch(url, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(init.headers || {}),
    },
    ...init,
    body: JSON.stringify(body),
  });
  const payload = await parseResponse(response);

  if (!response.ok) {
    throw new AnonymousIssuerRequestError(
      responseErrorMessage(payload, fallbackError),
      response.status,
      payload && typeof payload === 'object' ? payload.code : undefined
    );
  }

  if (payload && typeof payload === 'object' && 'data' in payload) {
    return payload.data;
  }

  return payload;
}

export function getAnonymousVPNMeta(serviceMeta) {
  if (!serviceMeta || typeof serviceMeta !== 'object') {
    return null;
  }

  const meta = serviceMeta.anonymousVpn;
  if (!meta || typeof meta !== 'object') {
    return null;
  }

  return meta;
}

export async function ensureAnonymousVPNIssuerEntitlement({
  apiUrl,
  anonymousMeta,
  address,
  allowDevRegistrationFallback = false,
  isConnected,
  signMessageAsync,
  identityCommitment,
  policyEpoch,
}) {
  if (!anonymousMeta?.issuer?.enabled) {
    if (allowDevRegistrationFallback) {
      return null;
    }
    throw new Error('Anonymous issuer is not available for this environment.');
  }

  if (
    !anonymousMeta.issuer.refresh ||
    !anonymousMeta.issuer.challenge ||
    !anonymousMeta.issuer.activate
  ) {
    throw new Error('Anonymous issuer metadata is incomplete.');
  }

  try {
    const refreshed = await postJSON(
      resolveEndpoint(apiUrl, anonymousMeta.issuer.refresh),
      {
        identityCommitment,
        policyEpoch,
      },
      { credentials: 'include' },
      'Failed to refresh anonymous credential'
    );

    return { mode: 'refreshed', result: refreshed };
  } catch (error) {
    if (!(error instanceof AnonymousIssuerRequestError) || error.status !== 401) {
      throw error;
    }
  }

  if (!isConnected || !address) {
    throw new Error(
      'Connect your wallet to activate anonymous paid access for this browser.'
    );
  }

  const challenge = await postJSON(
    resolveEndpoint(apiUrl, anonymousMeta.issuer.challenge),
    { address },
    {},
    'Failed to create anonymous issuer challenge'
  );
  const signature = await signMessageAsync({ message: challenge.message });
  const activated = await postJSON(
    resolveEndpoint(apiUrl, anonymousMeta.issuer.activate),
    {
      challengeToken: challenge.challengeToken,
      signature,
      identityCommitment,
      policyEpoch,
    },
    { credentials: 'include' },
    'Failed to activate anonymous paid access'
  );

  return { mode: 'activated', result: activated };
}
