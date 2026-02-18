# Sentinel dVPN Node -- Handshake Endpoint Analysis

> **Purpose**: This document traces the full `POST /` handshake endpoint in the Sentinel
> dVPN node codebase. A Go developer will use this to build NFT-gated access middleware
> that wraps the existing handshake handler.
>
> **Repositories analyzed** (both on `development` branch):
> - `github.com/sentinel-official/dvpn-node` (module: `github.com/sentinel-official/sentinel-dvpnx`)
> - `github.com/sentinel-official/sentinel-go-sdk` (module: `github.com/sentinel-official/sentinel-go-sdk`)
>
> **Go version**: 1.24.6 (dvpn-node), 1.24.0 (SDK)

---

## Table of Contents

1. [Repository Layout](#1-repository-layout)
2. [HTTP Route Definition](#2-http-route-definition)
3. [Request Payload](#3-request-payload)
4. [Handler Function -- Full Code Path](#4-handler-function----full-code-path)
5. [Response Payload](#5-response-payload)
6. [Service Interface (ServerService)](#6-service-interface-serverservice)
7. [WireGuard AddPeer Implementation](#7-wireguard-addpeer-implementation)
8. [Existing Authentication / Authorization](#8-existing-authentication--authorization)
9. [Node Configuration Format](#9-node-configuration-format)
10. [Server Setup and Middleware Chain](#10-server-setup-and-middleware-chain)
11. [Recommended NFT Middleware Insertion Point](#11-recommended-nft-middleware-insertion-point)
12. [Gotchas and Complications](#12-gotchas-and-complications)

---

## 1. Repository Layout

### dvpn-node (`sentinel-dvpnx`)

```
dvpn-node/
  api/
    routes.go                  # Top-level route registrar
    handshake/
      handlers.go              # POST / handler (THE key file)
      requests.go              # InitHandshakeRequest struct
      routes.go                # Registers POST / -> handlerInitHandshake
    info/
      ...                      # GET /info endpoint (status info)
  cmd/                         # CLI command definitions
  config/
    config.go                  # Config struct, validation, template rendering
    config.toml.tmpl           # TOML template with all settings
    handshake_dns.go           # HandshakeDNS config section
    node.go                    # NodeConfig -- ports, prices, intervals, service type
    oracle.go                  # Oracle config (price feeds)
    qos.go                     # QoS config (MaxPeers, max 250)
  core/
    context.go                 # core.Context -- holds all runtime state
    service.go                 # RemovePeerIfExists helper
    setup.go                   # Context.Setup() -- creates service, DB, clients
    tx.go                      # Transaction broadcasting
  database/
    database.go                # GORM/SQLite setup
    models/
      session.go               # Session model (DB schema)
    operations/
      ...                      # SessionFindOne, SessionInsertOne, etc.
  node/
    node.go                    # Node struct -- Start/Stop/Register/UpdateDetails
    setup.go                   # SetupServer() -- Gin router, middleware, TLS
  workers/                     # Background cron workers
  main.go                      # Entry point
  go.mod                       # Module: github.com/sentinel-official/sentinel-dvpnx
```

### sentinel-go-sdk

```
sentinel-go-sdk/
  types/
    service.go                 # ServerService interface (THE key interface)
    api.go                     # Response/Error types
    constants.go
    proto.go
  node/
    handshake.go               # InitHandshakeRequestBody, InitHandshakeResult, Client.InitHandshake()
    client.go                  # Node API client
    do.go                      # HTTP do() helper
    info.go                    # Info endpoint types
  wireguard/
    server.go                  # WireGuard ServerService implementation (AddPeer, RemovePeer, etc.)
    peer.go                    # Peer struct
    requests.go                # PeerRequest (WireGuard public key)
    responses.go               # AddPeerResponse
    metadata.go                # ServerMetadata (port, public key)
    ...
  v2ray/                       # V2Ray ServerService implementation
  openvpn/                     # OpenVPN ServerService implementation
  libs/
    gin/middlewares/
      rate-limiter.go          # The only existing Gin middleware
    cmux/                      # Connection multiplexer (TLS server)
    cron/                      # Scheduler
    ...
```

---

## 2. HTTP Route Definition

### File: `dvpn-node/api/handshake/routes.go`

```go
package handshake

import (
    "github.com/gin-gonic/gin"
    "github.com/sentinel-official/sentinel-dvpnx/core"
)

// RegisterRoutes registers the routes for the handshake API.
func RegisterRoutes(c *core.Context, r gin.IRouter) {
    r.POST("/", handlerInitHandshake(c))
}
```

This is called from `dvpn-node/api/routes.go`:

```go
package api

import (
    "github.com/gin-gonic/gin"
    "github.com/sentinel-official/sentinel-dvpnx/api/handshake"
    "github.com/sentinel-official/sentinel-dvpnx/api/info"
    "github.com/sentinel-official/sentinel-dvpnx/core"
)

func RegisterRoutes(c *core.Context, r gin.IRouter) {
    handshake.RegisterRoutes(c, r)
    info.RegisterRoutes(c, r)
}
```

Which is called from `dvpn-node/node/setup.go` in `SetupServer()`.

**Key takeaway**: The handshake is `POST /` -- it is the root POST endpoint. There is no path prefix.

---

## 3. Request Payload

### File: `sentinel-go-sdk/node/handshake.go`

The JSON body sent by the client:

```go
type InitHandshakeRequestBody struct {
    Data      []byte `binding:"required,gt=0"        json:"data"`
    ID        uint64 `binding:"required,gt=0"        json:"id"`
    PubKey    string `binding:"required,gt=0"        json:"pub_key"`
    Signature string `binding:"required,base64,gt=0" json:"signature"`
}
```

| Field       | Type     | Description |
|-------------|----------|-------------|
| `id`        | `uint64` | The blockchain session ID (on-chain, from Sentinel Hub) |
| `data`      | `[]byte` | Service-specific peer request data (JSON-encoded). For WireGuard, this is a `PeerRequest{PublicKey: <wg-pubkey>}` |
| `pub_key`   | `string` | The client's Cosmos SDK public key (used to derive the account address) |
| `signature` | `string` | Base64-encoded cryptographic signature over `bigEndian(id) || data` |

### Signature verification (`InitHandshakeRequestBody.Verify()`):

```go
func (r *InitHandshakeRequestBody) Verify() error {
    pubKey, err := utils.DecodePubKey(r.PubKey)
    if err != nil {
        return fmt.Errorf("decoding public key %q: %w", r.PubKey, err)
    }

    signature, err := base64.StdEncoding.DecodeString(r.Signature)
    if err != nil {
        return fmt.Errorf("decoding signature %q: %w", r.Signature, err)
    }

    if !pubKey.VerifySignature(r.Msg(), signature) {
        return fmt.Errorf("signature verification failed for session %d", r.ID)
    }

    return nil
}

func (r *InitHandshakeRequestBody) Msg() (buf []byte) {
    buf = append(buf, types.Uint64ToBigEndian(r.ID)...)
    buf = append(buf, r.Data...)
    return buf
}
```

The message that is signed is: `BigEndian(sessionID) || data_bytes`.

### Wrapped in dvpn-node's request type (`dvpn-node/api/handshake/requests.go`):

```go
type InitHandshakeRequest struct {
    Body node.InitHandshakeRequestBody
}

func NewInitHandshakeRequest(c *gin.Context) (req *InitHandshakeRequest, err error) {
    req = &InitHandshakeRequest{}
    if err := c.ShouldBindJSON(&req.Body); err != nil {
        return nil, fmt.Errorf("binding JSON request body: %w", err)
    }
    if err := req.Body.Verify(); err != nil {
        return nil, fmt.Errorf("verifying request body: %w", err)
    }
    return req, nil
}

func (r *InitHandshakeRequest) AccAddr() types.AccAddress {
    addr, err := r.Body.AccAddr()
    if err != nil {
        panic(fmt.Errorf("getting account addr from request body: %w", err))
    }
    return addr
}

func (r *InitHandshakeRequest) PeerRequest() []byte {
    return r.Body.Data
}
```

---

## 4. Handler Function -- Full Code Path

### File: `dvpn-node/api/handshake/handlers.go`

This is the core of the handshake. The function `handlerInitHandshake` is a closure that
captures `*core.Context` and returns a `gin.HandlerFunc`.

```go
func handlerInitHandshake(c *core.Context) gin.HandlerFunc {
    return func(ctx *gin.Context) {
        // ----- STEP 1: Check peer limit -----
        if n := c.Service().PeersLen(); uint(n) >= c.MaxPeers() {
            err := fmt.Errorf("maximum peer limit %d reached", n)
            ctx.JSON(http.StatusConflict, types.NewResponseError(1, err))
            return
        }

        // ----- STEP 2: Parse + verify request (signature check) -----
        req, err := NewInitHandshakeRequest(ctx)
        if err != nil {
            err = fmt.Errorf("parsing request from context: %w", err)
            ctx.JSON(http.StatusBadRequest, types.NewResponseError(2, err))
            return
        }

        // ----- STEP 3: Check for duplicate session by ID -----
        query := map[string]any{"id": req.Body.ID}
        record, err := operations.SessionFindOne(c.Database(), query)
        if err != nil {
            err = fmt.Errorf("retrieving session %d from database: %w", req.Body.ID, err)
            ctx.JSON(http.StatusInternalServerError, types.NewResponseError(3, err))
            return
        }
        if record != nil {
            err = fmt.Errorf("session %d already exists in database", req.Body.ID)
            ctx.JSON(http.StatusConflict, types.NewResponseError(3, err))
            return
        }

        // ----- STEP 4: Check for duplicate session by peer request -----
        peerReqStr := base64.StdEncoding.EncodeToString(req.PeerRequest())
        query = map[string]any{"peer_request": peerReqStr}
        record, err = operations.SessionFindOne(c.Database(), query)
        if err != nil {
            err = fmt.Errorf("retrieving session for peer request %q from database: %w", peerReqStr, err)
            ctx.JSON(http.StatusInternalServerError, types.NewResponseError(4, err))
            return
        }
        if record != nil {
            err = fmt.Errorf("session already exists for peer request %q", peerReqStr)
            ctx.JSON(http.StatusConflict, types.NewResponseError(4, err))
            return
        }

        // ----- STEP 5: Fetch session from blockchain -----
        session, err := c.Client().Session(ctx, req.Body.ID)
        if err != nil {
            err = fmt.Errorf("querying session %d from blockchain: %w", req.Body.ID, err)
            ctx.JSON(http.StatusInternalServerError, types.NewResponseError(5, err))
            return
        }
        if session == nil {
            err = fmt.Errorf("session %d does not exist on blockchain", req.Body.ID)
            ctx.JSON(http.StatusNotFound, types.NewResponseError(5, err))
            return
        }

        // ----- STEP 6: Validate session is active -----
        if !session.GetStatus().Equal(v1.StatusActive) {
            err = fmt.Errorf("invalid session status %q, expected %q", session.GetStatus(), v1.StatusActive)
            ctx.JSON(http.StatusBadRequest, types.NewResponseError(5, err))
            return
        }

        // ----- STEP 7: Validate node address matches -----
        if session.GetNodeAddress() != c.NodeAddr().String() {
            err = fmt.Errorf("node address mismatch: got %q, expected %q", session.GetNodeAddress(), c.NodeAddr())
            ctx.JSON(http.StatusBadRequest, types.NewResponseError(6, err))
            return
        }

        // ----- STEP 8: Validate account address (authorization) -----
        accAddr, err := cosmossdk.AccAddressFromBech32(session.GetAccAddress())
        if err != nil {
            err = fmt.Errorf("decoding Bech32 account addr %q: %w", session.GetAccAddress(), err)
            ctx.JSON(http.StatusInternalServerError, types.NewResponseError(6, err))
            return
        }
        if got := req.AccAddr(); !got.Equals(accAddr) {
            err = fmt.Errorf("account addr mismatch; got %q, expected %q", got, accAddr)
            ctx.JSON(http.StatusUnauthorized, types.NewResponseError(6, err))
            return
        }

        // ----- STEP 9: Add peer to VPN service (WireGuard/V2Ray) -----
        id, data, err := c.Service().AddPeer(ctx, req.PeerRequest())
        if err != nil {
            err = fmt.Errorf("adding peer to service: %w", err)
            ctx.JSON(http.StatusInternalServerError, types.NewResponseError(7, err))
            return
        }

        // ----- STEP 10: Build response -----
        res := &node.InitHandshakeResult{Addrs: c.RemoteAddrs()}
        if res.Data, err = json.Marshal(data); err != nil {
            err = fmt.Errorf("encoding add-peer service response: %w", err)
            ctx.JSON(http.StatusInternalServerError, types.NewResponseError(8, err))
            return
        }

        // ----- STEP 11: Persist session to database -----
        item := models.NewSession().
            WithAccAddr(accAddr).
            WithDuration(0).
            WithID(session.GetID()).
            WithMaxBytes(session.GetMaxBytes()).
            WithMaxDuration(session.GetMaxDuration()).
            WithNodeAddr(c.NodeAddr()).
            WithPeerID(id).
            WithPeerMetadata(res.Data).
            WithPeerRequest(req.PeerRequest()).
            WithRxBytes(math.ZeroInt()).
            WithServiceType(c.Service().Type()).
            WithSignature(nil).
            WithTxBytes(math.ZeroInt())

        if err = operations.SessionInsertOne(c.Database(), item); err != nil {
            err = fmt.Errorf("inserting session %d into database: %w", item.GetID(), err)
            ctx.JSON(http.StatusInternalServerError, types.NewResponseError(9, err))
            return
        }

        // ----- STEP 12: Return success -----
        ctx.JSON(http.StatusOK, types.NewResponseResult(res))
    }
}
```

### Execution flow summary:

```
Client POST / with JSON body
    |
    v
[CORS middleware]
    |
    v
[Rate limiter middleware]
    |
    v
handlerInitHandshake()
    |
    +-- (1) Check peer count < MaxPeers
    +-- (2) Parse JSON body + verify cryptographic signature
    +-- (3) Check no duplicate session by ID in local DB
    +-- (4) Check no duplicate session by peer request in local DB
    +-- (5) Query blockchain for session, confirm it exists
    +-- (6) Confirm session status == Active
    +-- (7) Confirm session.NodeAddress == this node's address
    +-- (8) Confirm request's account address == session's account address
    +-- (9) Call Service.AddPeer() -- sets up WireGuard tunnel
    +-- (10) Marshal response (node addresses + peer config)
    +-- (11) Insert session record into SQLite
    +-- (12) Return 200 OK with result
```

---

## 5. Response Payload

### File: `sentinel-go-sdk/node/handshake.go`

```go
type InitHandshakeResult struct {
    Addrs []string `json:"addrs"` // Node's remote addresses (IPv4, IPv6, domain)
    Data  []byte   `json:"data"`  // JSON-encoded service-specific response
}
```

Wrapped in the standard response envelope (`sentinel-go-sdk/types/api.go`):

```go
type Response struct {
    Success bool   `json:"success"`
    Error   *Error `json:"error,omitempty"`
    Result  any    `json:"result,omitempty"`
}
```

### Success response (200 OK):

```json
{
    "success": true,
    "result": {
        "addrs": ["1.2.3.4:51820", "[::1]:51820"],
        "data": "<base64 JSON of AddPeerResponse>"
    }
}
```

### For WireGuard, `data` decodes to (`sentinel-go-sdk/wireguard/responses.go`):

```go
type AddPeerResponse struct {
    Addrs    []*netip.Prefix   `json:"addrs"`    // Allocated IP addresses for the peer
    Metadata []*ServerMetadata `json:"metadata"`  // Server port + public key
}

// ServerMetadata (sentinel-go-sdk/wireguard/metadata.go):
type ServerMetadata struct {
    Port      uint16 `json:"port"`       // WireGuard listen port
    PublicKey *Key   `json:"public_key"` // Server's WireGuard public key
}
```

### Error response (4xx/5xx):

```json
{
    "success": false,
    "error": {
        "code": 6,
        "message": "account addr mismatch; got \"sent1abc...\" expected \"sent1xyz...\""
    }
}
```

Error codes in the handler:
| Code | HTTP Status | Meaning |
|------|-------------|---------|
| 1    | 409         | Max peer limit reached |
| 2    | 400         | Request parse/signature verification failed |
| 3    | 500/409     | Session lookup by ID failed / duplicate |
| 4    | 500/409     | Session lookup by peer request failed / duplicate |
| 5    | 500/404/400 | Blockchain session query failed / not found / not active |
| 6    | 400/500/401 | Node addr mismatch / addr decode error / account addr mismatch |
| 7    | 500         | AddPeer failed |
| 8    | 500         | Response encoding failed |
| 9    | 500         | Database insert failed |

---

## 6. Service Interface (ServerService)

### File: `sentinel-go-sdk/types/service.go`

```go
// BaseService defines the common behavior shared by all services.
type BaseService interface {
    Type() ServiceType
    IsRunning() (bool, error)
    Init(force bool) error
    Setup(ctx context.Context) error
    Start(parent context.Context) (context.Context, error)
    Stop() error
    Wait(ctx context.Context) error
    Cleanup() error
}

// ServerService defines the interface for server-side service operations.
type ServerService interface {
    BaseService
    AddPeer(ctx context.Context, req any) (id string, res any, err error)
    HasPeer(ctx context.Context, id string) (bool, error)
    RemovePeer(ctx context.Context, id string) error
    PeersLen() int
    PeerStatistics() (map[string]*PeerStatistics, error)
}
```

### Method details:

| Method | Signature | Description |
|--------|-----------|-------------|
| `AddPeer` | `(ctx, req any) -> (id string, res any, err)` | Adds a new VPN peer. `req` is the service-specific request (e.g., WireGuard public key). Returns peer ID and service-specific response. |
| `HasPeer` | `(ctx, id string) -> (bool, err)` | Checks if a peer exists by its ID (e.g., WireGuard public key string). |
| `RemovePeer` | `(ctx, id string) -> err` | Removes a peer, deallocates IPs, removes from WireGuard. |
| `PeersLen` | `() -> int` | Returns current number of connected peers. Used for the MaxPeers check. |
| `PeerStatistics` | `() -> (map[string]*PeerStatistics, err)` | Returns bandwidth stats for all peers. Used by background workers for billing. |

### PeerStatistics struct:

```go
type PeerStatistics struct {
    CreatedAt time.Time `json:"created_at,omitzero"`
    UpdatedAt time.Time `json:"updated_at,omitzero"`
    RxBytes   int64     `json:"rx_bytes,omitempty"`
    TxBytes   int64     `json:"tx_bytes,omitempty"`
}
```

### ServiceType enum:

```go
type ServiceType byte

const (
    ServiceTypeUnspecified ServiceType = 0x00 + iota
    ServiceTypeWireGuard
    ServiceTypeV2Ray
    ServiceTypeOpenVPN
)
```

---

## 7. WireGuard AddPeer Implementation

### File: `sentinel-go-sdk/wireguard/server.go`

```go
type Server struct {
    *process.Manager
    cfg      *ServerConfig
    device   string
    homeDir  string
    metadata []*ServerMetadata
    peers    *safe.Map[string, Peer]
    pools    *netip.AddrPoolSet
}

func (s *Server) AddPeer(ctx context.Context, req any) (string, any, error) {
    // 1. Parse the request (accepts []byte JSON or *PeerRequest)
    r, err := parsePeerRequest(req)
    if err != nil {
        return "", nil, fmt.Errorf("parsing request: %w", err)
    }

    // 2. Validate (public key must be non-nil and non-zero)
    if err := r.Validate(); err != nil {
        return "", nil, fmt.Errorf("validating request: %w", err)
    }

    // 3. ID is the WireGuard public key string
    id := r.ID()

    // 4. Acquire IP addresses from the pool
    addrs, err := s.pools.Acquire()
    if err != nil {
        return "", nil, fmt.Errorf("acquiring peer %q addrs: %w", id, err)
    }

    // Deferred cleanup: release addrs if peer wasn't actually added
    defer func() {
        if ok := s.peers.Exists(id); !ok {
            if err := s.pools.Release(addrs); err != nil {
                panic(fmt.Errorf("releasing peer %q addrs %v: %w", id, addrs, err))
            }
        }
    }()

    // 5. Build allowed-IPs list (/32 for IPv4, /128 for IPv6)
    allowedIPs := make([]string, 0, len(addrs))
    prefixAddrs := make([]*netip.Prefix, 0, len(addrs))
    for _, addr := range addrs {
        b := 32
        if addr.Is6() {
            b = 128
        }
        prefix, err := addr.Prefix(b)
        if err != nil {
            return "", nil, fmt.Errorf("getting prefix: %w", err)
        }
        allowedIPs = append(allowedIPs, prefix.String())
        prefixAddrs = append(prefixAddrs, &netip.Prefix{Prefix: prefix})
    }

    // 6. Execute: wg set <device> peer <pubkey> allowed-ips <ips>
    cmd := exec.CommandContext(ctx, s.execFile("wg"),
        "set", s.device, "peer", id, "allowed-ips", strings.Join(allowedIPs, ","),
    )
    if err := cmd.Run(); err != nil {
        return "", nil, fmt.Errorf("running command: %w", err)
    }

    // 7. Store peer in thread-safe map
    now := time.Now()
    s.peers.Set(id, Peer{
        ID:       id,
        Addrs:    addrs,
        Current:  types.NewPeerStatistics(now),
        Previous: types.NewPeerStatistics(now),
    })

    // 8. Return ID + response containing allocated IPs and server metadata
    return id, &AddPeerResponse{
        Addrs:    prefixAddrs,
        Metadata: s.metadata,
    }, nil
}
```

### WireGuard PeerRequest (`sentinel-go-sdk/wireguard/requests.go`):

```go
type PeerRequest struct {
    PublicKey *Key `json:"public_key"`
}
```

The `Data` field in the handshake request body is JSON-encoded `PeerRequest`. So a WireGuard
handshake request's `data` field looks like:

```json
{"public_key": "base64-encoded-wg-pubkey"}
```

---

## 8. Existing Authentication / Authorization

The node performs the following auth checks (all inside `handlerInitHandshake`):

### 8.1 Cryptographic Signature Verification (Step 2)

```go
req, err := NewInitHandshakeRequest(ctx)
// This calls req.Body.Verify() which:
//   1. Decodes the Cosmos SDK public key from req.PubKey
//   2. Decodes the base64 signature from req.Signature
//   3. Verifies: pubKey.VerifySignature(BigEndian(ID) || Data, signature)
```

**What this proves**: The requester holds the private key corresponding to the `pub_key` field.

### 8.2 Blockchain Session Validation (Steps 5-7)

```go
session, err := c.Client().Session(ctx, req.Body.ID)
// Checks:
//   - Session exists on-chain
//   - Session status == Active
//   - Session's node address == this node's address
```

**What this proves**: An active, paid-for session exists on the Sentinel blockchain, assigned
to this specific node.

### 8.3 Account Address Authorization (Step 8)

```go
accAddr, _ := cosmossdk.AccAddressFromBech32(session.GetAccAddress())
if got := req.AccAddr(); !got.Equals(accAddr) {
    // 401 Unauthorized
}
```

**What this proves**: The person making the request (whose public key signed the message)
is the same person who created the blockchain session (whose account address is recorded
on-chain).

### 8.4 What is NOT checked

- **No NFT ownership check** -- the node does not query any NFT contract.
- **No subscription/plan check** -- beyond the on-chain session, there is no secondary authorization.
- **No IP allowlisting** -- any IP can connect.
- **No API key or bearer token** -- authentication is purely cryptographic (Cosmos SDK signatures).
- **No deposit/stake check by the node** -- that is handled by the blockchain when creating the session.

### Summary of existing auth:

```
Request signed by client's private key
    -> Signature verified locally
    -> Session ID looked up on Sentinel blockchain
    -> Session must be Active
    -> Session must be assigned to this node
    -> Session's account address must match the signer's address
```

This is a strong authentication scheme. Our NFT middleware will add an **additional
authorization layer** on top of it, not replace it.

---

## 9. Node Configuration Format

### File: `dvpn-node/config/config.toml.tmpl`

The configuration is a TOML file with these sections:

```toml
# --- Keyring ---
[keyring]
backend = "file"           # Options: file, kwallet, memory, os, pass, test

# --- RPC Query ---
[query]
proof = true
max_retries = 10

# --- RPC Endpoints ---
[rpc]
addrs = ["https://rpc.sentinel.co:443"]
chain_id = "sentinelhub-2"
timeout = "15s"

# --- Transaction Settings ---
[tx]
fee_denom = "udvpn"
fee_amount = 100000
gas = 200000
gas_adjustment = 1.5
max_retries = 10

# --- Handshake DNS ---
[handshake_dns]
enabled = false
peers = 8                  # max 8

# --- Node Settings ---
[node]
api_port = 7777            # API listen port
moniker = "my-node"
remote_addrs = ["1.2.3.4:7777"]
service_type = "wireguard" # Options: wireguard, v2ray, openvpn

# Pricing
gigabyte_prices = "1000000udvpn"
hourly_prices = "1000000udvpn"

# Background worker intervals
interval_best_rpc_addr = "30m"
interval_geo_ip_location = "1h"
interval_prices_update = "6h"
interval_session_usage_sync_with_blockchain = "5m"
interval_session_usage_sync_with_database = "1m"
interval_session_usage_validate = "1m"
interval_session_validate = "1m"
interval_speedtest = "6h"
interval_status_update = "1h"

# --- Oracle ---
[oracle]
source = "coingecko"       # Options: coingecko, osmosis

# --- QoS ---
[qos]
max_peers = 250            # Maximum: 250
```

### Key config structs:

**QoSConfig** (`config/qos.go`):
```go
const MaxQoSMaxPeers = 250

type QoSConfig struct {
    MaxPeers uint
}
```

**NodeConfig** (`config/node.go`) -- 15 fields covering API port, moniker, remote addresses,
service type, pricing, and all background worker intervals.

---

## 10. Server Setup and Middleware Chain

### File: `dvpn-node/node/setup.go` -- `SetupServer()`

```go
func (n *Node) SetupServer(ctx context.Context, _ *config.Config) error {
    gin.SetMode(gin.ReleaseMode)

    items := []gin.HandlerFunc{
        cors.New(cors.Config{
            AllowAllOrigins: true,
            AllowMethods:    []string{http.MethodGet, http.MethodPost},
        }),
        middlewares.RateLimiter(ctx, nil),
    }

    router := gin.New()
    router.Use(items...)

    api.RegisterRoutes(n.Context(), router)

    s := cmux.NewServer(
        "API-server",
        n.Context().APIListenAddr(),
        n.Context().TLSCertFile(),
        n.Context().TLSKeyFile(),
        router,
    )
    if err := s.Setup(ctx); err != nil {
        return err
    }

    n.WithServer(s)
    return nil
}
```

**Current middleware chain**:

```
Request
  -> CORS middleware
  -> Rate limiter middleware
  -> Route handler (handlerInitHandshake for POST /)
```

The only middleware from the SDK is a rate limiter (`sentinel-go-sdk/libs/gin/middlewares/rate-limiter.go`). There is no authentication middleware -- all auth is done inside the handler itself.

---

## 11. Recommended NFT Middleware Insertion Point

### Option A: Gin Middleware (RECOMMENDED)

Insert a Gin middleware **after the rate limiter but before route registration**. This is the
cleanest approach and requires minimal changes to the existing codebase.

**Where to modify**: `dvpn-node/node/setup.go` in `SetupServer()`

```go
func (n *Node) SetupServer(ctx context.Context, _ *config.Config) error {
    gin.SetMode(gin.ReleaseMode)

    items := []gin.HandlerFunc{
        cors.New(cors.Config{
            AllowAllOrigins: true,
            AllowMethods:    []string{http.MethodGet, http.MethodPost},
        }),
        middlewares.RateLimiter(ctx, nil),
        // >>> INSERT NFT MIDDLEWARE HERE <<<
        nftgate.Middleware(n.Context(), nftGateConfig),
    }

    router := gin.New()
    router.Use(items...)

    api.RegisterRoutes(n.Context(), router)
    // ...
}
```

**The middleware would**:

1. Only intercept `POST /` requests (skip `GET /info` and other methods)
2. Parse the request body (need to buffer it since `gin.Context.Request.Body` is read-once)
3. Extract `pub_key` from the JSON body
4. Derive the Cosmos account address from the public key
5. Query the NFT contract (on Cosmos or EVM chain) to check if that address owns the required NFT
6. If NFT check passes: restore the request body and call `ctx.Next()`
7. If NFT check fails: return `403 Forbidden` with an appropriate error

**Important**: You must buffer and restore `ctx.Request.Body` because the downstream handler
also reads it. Use `io.ReadAll` + `io.NopCloser(bytes.NewReader(...))`.

### Option B: Wrap the Handler

Modify `dvpn-node/api/handshake/routes.go` to wrap the handler:

```go
func RegisterRoutes(c *core.Context, r gin.IRouter) {
    r.POST("/", nftgate.Wrap(c, handlerInitHandshake(c)))
}
```

This is more targeted but couples the NFT logic to the handshake package.

### Option C: Intercept Inside the Handler

Add NFT check logic directly into `handlerInitHandshake()` after Step 2 (request parsing)
but before Step 5 (blockchain query). This avoids body buffering but is the most invasive.

```go
// After Step 2 in handlers.go:
req, err := NewInitHandshakeRequest(ctx)
// ...

// >>> INSERT NFT CHECK HERE <<<
if !nftgate.CheckOwnership(ctx, req.AccAddr()) {
    ctx.JSON(http.StatusForbidden, types.NewResponseError(10, "NFT required"))
    return
}

// Continue with Step 3...
```

### Recommendation

**Use Option A (Gin middleware)** for maximum separation of concerns. The middleware is
self-contained, can be enabled/disabled via config, and does not require modifying any
existing Sentinel code files beyond `SetupServer()`.

If body buffering is a concern, **use Option C** -- inserting a check inside the handler
right after the request is parsed (after Step 2, before Step 3). At that point you have
the parsed `req.AccAddr()` available and haven't done any expensive blockchain queries yet.

---

## 12. Gotchas and Complications

### 12.1 Request Body Read-Once Problem (Option A)

Gin's `ShouldBindJSON` reads `ctx.Request.Body` which is an `io.ReadCloser` -- it can only
be read once. If your middleware reads the body to extract the public key, you MUST restore
it before calling `ctx.Next()`:

```go
func Middleware(...) gin.HandlerFunc {
    return func(ctx *gin.Context) {
        bodyBytes, _ := io.ReadAll(ctx.Request.Body)
        ctx.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

        // Parse body, check NFT...

        ctx.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
        ctx.Next()
    }
}
```

### 12.2 The `req any` Interface in AddPeer

`ServerService.AddPeer(ctx, req any)` accepts `any`. For WireGuard, it expects either
`[]byte` (JSON) or `*wireguard.PeerRequest`. The handler passes `req.PeerRequest()` which
returns `[]byte` (the raw `Data` field from the request body). This is important if you
ever need to intercept at the service level.

### 12.3 Session ID is On-Chain

The `id` field in the request is a blockchain session ID. The client must have already
created a session on the Sentinel Hub blockchain (which requires a deposit). The node
verifies this. Your NFT gate adds a SECOND requirement but does NOT replace the on-chain
session requirement.

### 12.4 Account Address Derivation

To derive the Cosmos account address from the public key in the middleware:

```go
import "github.com/sentinel-official/sentinel-go-sdk/utils"

pubKey, err := utils.DecodePubKey(body.PubKey)
accAddr := pubKey.Address().Bytes()
// accAddr is a types.AccAddress ([]byte)
// Bech32: cosmossdk.AccAddress(accAddr).String() -> "sent1..."
```

### 12.5 TLS Required

The server uses TLS (cmux.NewServer takes cert/key files). The NFT middleware runs
inside the TLS-terminated HTTP handler, so this is transparent, but clients must connect
via HTTPS.

### 12.6 Module Path vs Repo Name

The repository is `github.com/sentinel-official/dvpn-node` but the Go module is
`github.com/sentinel-official/sentinel-dvpnx`. Import paths use the module name, not
the repo name.

### 12.7 MaxPeers Check Happens First

The peer limit check (Step 1) happens before any request parsing. This means even NFT
holders will be rejected if the node is full. This is correct behavior -- the NFT gate
should not override capacity limits.

### 12.8 No Middleware Authentication Exists

The existing codebase has NO authentication middleware. All auth is inside the handler.
This means adding the first real middleware is a design precedent. Keep it clean.

### 12.9 Background Workers Validate Sessions

Workers in `dvpn-node/workers/` periodically:
- Sync session usage with the blockchain (billing)
- Validate session status (remove expired/inactive sessions)
- Run speed tests

These workers call `RemovePeer()` when sessions expire. Your NFT middleware only gates
initial access -- ongoing session management is handled by the existing worker system.

### 12.10 Database is SQLite via GORM

Sessions are stored in a local SQLite database via GORM. The `Session` model includes
all the fields from the handshake (account address, peer ID, bandwidth limits, etc.).
If you need to store NFT verification results, you could add a field to the Session model
or create a separate table.

---

## Quick Reference: Key File Locations

| What | Repository | File Path |
|------|-----------|-----------|
| Route registration | dvpn-node | `api/handshake/routes.go` |
| Handshake handler | dvpn-node | `api/handshake/handlers.go` |
| Request struct | dvpn-node | `api/handshake/requests.go` |
| Top-level routes | dvpn-node | `api/routes.go` |
| Server setup (middleware) | dvpn-node | `node/setup.go` |
| Core context | dvpn-node | `core/context.go` |
| Service setup | dvpn-node | `core/setup.go` |
| Config template | dvpn-node | `config/config.toml.tmpl` |
| QoS config | dvpn-node | `config/qos.go` |
| Session DB model | dvpn-node | `database/models/session.go` |
| ServerService interface | sentinel-go-sdk | `types/service.go` |
| Request/Response types | sentinel-go-sdk | `node/handshake.go` |
| API response envelope | sentinel-go-sdk | `types/api.go` |
| WireGuard server | sentinel-go-sdk | `wireguard/server.go` |
| WireGuard peer request | sentinel-go-sdk | `wireguard/requests.go` |
| WireGuard response | sentinel-go-sdk | `wireguard/responses.go` |
| Rate limiter middleware | sentinel-go-sdk | `libs/gin/middlewares/rate-limiter.go` |

---

## Appendix: Example Handshake Request/Response

### Request (POST /):

```json
{
    "id": 12345,
    "data": "eyJwdWJsaWNfa2V5IjoiYmFzZTY0LWVuY29kZWQtd2ctcHVia2V5In0=",
    "pub_key": "cosmos-sdk-encoded-public-key",
    "signature": "base64-encoded-signature-over-bigendian(12345)||data"
}
```

Where `data` base64-decodes to:
```json
{"public_key": "base64-encoded-wg-pubkey"}
```

### Response (200 OK):

```json
{
    "success": true,
    "result": {
        "addrs": ["1.2.3.4:51820"],
        "data": "eyJhZGRycyI6WyIxMC44LjAuMi8zMiJdLCJtZXRhZGF0YSI6W3sicG9ydCI6NTE4MjAsInB1YmxpY19rZXkiOiJzZXJ2ZXItd2ctcHVia2V5In1dfQ=="
    }
}
```

Where `result.data` base64-decodes to:
```json
{
    "addrs": ["10.8.0.2/32"],
    "metadata": [{"port": 51820, "public_key": "server-wg-pubkey"}]
}
```

The client uses this to configure their local WireGuard interface:
- Peer endpoint: `1.2.3.4:51820`
- Peer public key: `server-wg-pubkey`
- Local address: `10.8.0.2/32`
