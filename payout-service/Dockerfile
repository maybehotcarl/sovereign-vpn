# ===========================================================================
# Sovereign VPN - Payout Service
# Multi-stage Docker build for Node.js 20 + TypeScript
# ===========================================================================

# ---------------------------------------------------------------------------
# Stage 1: Install dependencies and build TypeScript
# ---------------------------------------------------------------------------
FROM node:20-alpine AS builder

WORKDIR /app

# Install build dependencies for better-sqlite3 and leveldown (native addons)
RUN apk add --no-cache python3 make g++

# Copy package files first for layer caching
COPY package.json package-lock.json* ./

# Install all dependencies (including devDependencies for building)
RUN npm ci

# Copy source code
COPY tsconfig.json ./
COPY src/ ./src/

# Build TypeScript
RUN npm run build

# Prune devDependencies
RUN npm prune --production

# ---------------------------------------------------------------------------
# Stage 2: Production runtime
# ---------------------------------------------------------------------------
FROM node:20-alpine AS runtime

WORKDIR /app

# better-sqlite3 and leveldown need these at runtime
RUN apk add --no-cache libstdc++

# Create data directories for SQLite, RAILGUN wallet state, and artifacts
RUN mkdir -p /app/data /app/data/railgun-db /app/data/railgun-artifacts \
    && chown -R node:node /app/data

# Copy production node_modules from builder
COPY --from=builder /app/node_modules ./node_modules

# Copy compiled JavaScript
COPY --from=builder /app/dist ./dist

# Copy package.json for metadata
COPY --from=builder /app/package.json ./

# Run as non-root user
USER node

# Default environment variables
ENV NODE_ENV=production
ENV RECEIPT_DB_PATH=/app/data/receipts.db
ENV RAILGUN_DB_PATH=/app/data/railgun-db
ENV RAILGUN_ARTIFACTS_PATH=/app/data/railgun-artifacts
ENV HEALTH_PORT=3001

# Expose the health check port
EXPOSE 3001

# Persistent volumes for RAILGUN state and artifacts
VOLUME ["/app/data"]

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:3001/health || exit 1

# Start the service
CMD ["node", "dist/index.js"]
