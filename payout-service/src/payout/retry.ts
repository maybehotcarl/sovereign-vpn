// ---------------------------------------------------------------------------
// Exponential Backoff Retry Utility
// ---------------------------------------------------------------------------

/**
 * Execute an async function with exponential backoff retry logic.
 *
 * On each failure the delay doubles (with jitter) up to a maximum.
 * This is useful for on-chain transaction submission where transient
 * RPC errors or nonce conflicts can occur.
 *
 * @param fn           The async function to execute
 * @param maxRetries   Maximum number of retry attempts (default: 3)
 * @param baseDelayMs  Initial delay in milliseconds before first retry (default: 1000)
 * @param maxDelayMs   Maximum delay cap in milliseconds (default: 30000)
 * @returns The return value of `fn` on success
 * @throws The last error encountered if all retries are exhausted
 */
export async function withRetry<T>(
  fn: () => Promise<T>,
  maxRetries: number = 3,
  baseDelayMs: number = 1000,
  maxDelayMs: number = 30_000,
): Promise<T> {
  let lastError: unknown;

  for (let attempt = 0; attempt <= maxRetries; attempt++) {
    try {
      return await fn();
    } catch (err) {
      lastError = err;

      if (attempt >= maxRetries) {
        break;
      }

      // Exponential backoff: baseDelay * 2^attempt
      const exponentialDelay = baseDelayMs * Math.pow(2, attempt);
      // Add jitter: random value between 0 and half the exponential delay
      const jitter = Math.random() * (exponentialDelay / 2);
      const delay = Math.min(exponentialDelay + jitter, maxDelayMs);

      console.warn(
        `[retry] Attempt ${attempt + 1}/${maxRetries + 1} failed. ` +
        `Retrying in ${Math.round(delay)}ms...`,
        err instanceof Error ? err.message : err,
      );

      await sleep(delay);
    }
  }

  console.error(
    `[retry] All ${maxRetries + 1} attempts failed. Throwing last error.`,
  );
  throw lastError;
}

/**
 * Sleep for the specified number of milliseconds.
 */
function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
