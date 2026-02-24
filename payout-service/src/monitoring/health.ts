import express from "express";
import type { Server } from "node:http";

// ---------------------------------------------------------------------------
// Health Check HTTP Server
// ---------------------------------------------------------------------------

export interface HealthStatus {
  /** Whether the service is running */
  running: boolean;
  /** ISO timestamp of the last completed payout cycle (null if never run) */
  lastRunAt: string | null;
  /** ISO timestamp of the next scheduled payout cycle (null if not scheduled) */
  nextRunAt: string | null;
  /** Number of operators with pending payouts above threshold */
  pendingOperatorCount: number;
  /** Whether the service is in dry-run mode */
  dryRun: boolean;
  /** Service uptime in seconds */
  uptimeSeconds: number;
}

/**
 * Start a lightweight Express HTTP server exposing a GET /health endpoint.
 *
 * The `getStatus` callback is invoked on each request to fetch the current
 * service state. This avoids tight coupling between the health server and
 * the payout processor.
 *
 * @param port       TCP port to listen on
 * @param getStatus  Callback that returns the current health status
 * @returns The HTTP server instance (for graceful shutdown)
 */
export function startHealthServer(
  port: number,
  getStatus: () => HealthStatus,
): Server {
  const app = express();
  const startTime = Date.now();

  app.get("/health", (_req, res) => {
    try {
      const status = getStatus();
      status.uptimeSeconds = Math.floor((Date.now() - startTime) / 1000);

      // Return 200 if the service is running, 503 otherwise
      const httpStatus = status.running ? 200 : 503;
      res.status(httpStatus).json(status);
    } catch (err) {
      console.error("[health] Error generating health status:", err);
      res.status(500).json({
        running: false,
        error: "Failed to generate health status",
      });
    }
  });

  // Simple readiness/liveness for container orchestrators
  app.get("/ready", (_req, res) => {
    res.status(200).json({ ready: true });
  });

  const server = app.listen(port, () => {
    console.log(`[health] Health server listening on port ${port}`);
  });

  return server;
}
