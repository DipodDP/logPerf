# iperf2 Bug Report: Windows UDP server fails after first client (WSAECONNRESET)

## Title

Windows UDP server becomes unusable after first client disconnects — missing SIO_UDP_CONNRESET fix

## Summary

On Windows, an iperf2 UDP server (`-s -u`) cannot deliver Server Report ACKs to any client after the first one disconnects. The server-side output shows `"recvfrom failed: Connection reset by peer"` and all subsequent clients receive `"WARNING: did not receive ack of last datagram after 10 tries"`. This affects both daemon mode (`-D`) and foreground mode with persistent servers.

The root cause is a well-documented Windows platform behavior (Microsoft KB 263823): when a UDP socket sends a packet to a closed destination port, Windows delivers the resulting ICMP "Port Unreachable" as `WSAECONNRESET` (error 10054) on the next `recvfrom()` call. The standard fix — `WSAIoctl(SIO_UDP_CONNRESET)` — is applied by Go, Rust (tokio/mio), .NET, pjsip, and many other networking libraries, but is missing from iperf2.

## Environment

- **Server:** Windows 10/11, iperf2 2.2.1 (iperf-2.2.1-win64.exe)
- **Client:** macOS 15, iperf2 2.2.1
- **Network:** Tailscale VPN (directly routable, no NAT)
- **Affected modes:** `-s -u` (foreground), `-s -u -D` (daemon), all `-P` counts, all bandwidths

## Steps to Reproduce

```bash
# On Windows — start persistent UDP server
iperf.exe -s -u -p 5201 -D

# On Linux/macOS — run client twice
iperf -c <windows-ip> -u -p 5201 -t 3 -b 1M    # Run 1: Server Report received ✓
iperf -c <windows-ip> -u -p 5201 -t 3 -b 1M    # Run 2: WARNING: did not receive ack ✗
```

## Expected Behavior

Both clients receive Server Reports with valid jitter/loss statistics.

## Actual Behavior

- **Run 1:** Server Report delivered correctly
- **Run 2+:** `WARNING: did not receive ack of last datagram after 10 tries`
- **Server stderr:** `recvfrom failed: Connection reset by peer`

## Test Evidence

Tested with 12 consecutive clients against a persistent daemon, varying parameters:

| Parameters | Server Reports received | WARNINGs |
|---|---|---|
| -P 4 -b 1M (3 runs) | 0 | 12 |
| -P 2 -b 2M (3 runs) | 0 | 6 |
| -P 1 -b 4M (3 runs) | 0 | 3 |
| -P 4 -b 500K (3 runs) | 0 | 12 |

100% ACK failure after the first client, independent of stream count and bandwidth.

## Root Cause Analysis

### The Windows WSAECONNRESET mechanism

1. Client finishes test and closes its ephemeral UDP port
2. Server sends Server Report ACK to that (now-closed) port
3. Client OS responds with ICMP "Port Unreachable"
4. **Windows delivers the ICMP error as `WSAECONNRESET` (10054) on the server's UDP socket** — this does NOT happen on Linux/macOS, where ICMP errors on UDP sockets are silently ignored
5. On the next `recvfrom()` call (for client #2), the stale `WSAECONNRESET` is returned instead of data

### Why iperf2 is affected

In `include/util.h`, the `FATALUDPREADERR` macro on Windows treats ALL errors except `WSAEWOULDBLOCK` as fatal:

```c
// Windows — WSAECONNRESET (10054) is treated as fatal
#define FATALUDPREADERR(errno) (((errno = WSAGetLastError()) != WSAEWOULDBLOCK))

// Unix — correctly excludes transient errors (EAGAIN, EWOULDBLOCK, EINTR)
#define FATALUDPREADERR(errno) ((errno != EAGAIN) && (errno != EWOULDBLOCK) && (errno != EINTR))
```

When `WSAECONNRESET` hits, `Server.cpp` sets `peerclose = true` and the server thread exits. The socket is permanently poisoned.

### The standard platform fix

Microsoft documents this behavior in KB 263823 and provides `SIO_UDP_CONNRESET` (available since Windows XP) to disable it:

```c
BOOL bNewBehavior = FALSE;
DWORD dwBytesReturned = 0;
WSAIoctl(sock, SIO_UDP_CONNRESET, &bNewBehavior, sizeof(bNewBehavior),
         NULL, 0, &dwBytesReturned, NULL, NULL);
```

### Projects that apply this fix

| Project | Fix |
|---|---|
| Go `net` package | `WSAIoctl(SIO_UDP_CONNRESET)` on all UDP sockets ([golang/go#5834](https://github.com/golang/go/issues/5834)) |
| Rust `mio` / `tokio` | `WSAIoctl(SIO_UDP_CONNRESET)` ([tokio#2017](https://github.com/tokio-rs/tokio/issues/2017)) |
| .NET runtime | Applied by default on all UDP sockets |
| pjsip | `WSAIoctl(SIO_UDP_CONNRESET)` ([pjsip#1197](https://github.com/pjsip/pjproject/issues/1197)) |
| Dart/Flutter | `WSAIoctl(SIO_UDP_CONNRESET)` ([flutter#155823](https://github.com/flutter/flutter/issues/155823)) |

## Proposed Patch

Two complementary changes:

### 1. Suppress WSAECONNRESET at socket creation (primary fix)

Add `WSAIoctl(SIO_UDP_CONNRESET)` after UDP socket creation in `Listener.cpp` and `Client.cpp`.

**`include/headers.h`** — add `<mstcpip.h>` or define the constant:
```c
#ifdef WIN32
// ... existing includes ...
#ifndef SIO_UDP_CONNRESET
#define SIO_UDP_CONNRESET _WSAIOW(IOC_VENDOR, 12)
#endif
#endif
```

**`src/Listener.cpp`** — after `mSettings->mSock = ListenSocket;` (before `SetSocketOptions`):
```c
#ifdef WIN32
    if (isUDP(mSettings)) {
        // Disable Windows WSAECONNRESET on UDP sockets (KB 263823).
        // Without this, ICMP Port Unreachable from a disconnected client
        // poisons the socket, causing all subsequent recvfrom() calls to
        // fail with WSAECONNRESET (error 10054).
        BOOL bNewBehavior = FALSE;
        DWORD dwBytesReturned = 0;
        WSAIoctl(ListenSocket, SIO_UDP_CONNRESET, &bNewBehavior,
                 sizeof(bNewBehavior), NULL, 0, &dwBytesReturned, NULL, NULL);
    }
#endif
```

**`src/Client.cpp`** — same pattern after `mSettings->mSock = mySocket;`.

### 2. Exclude WSAECONNRESET from FATALUDPREADERR (secondary safety net)

**`include/util.h`** — update the Windows macro:
```c
// Before:
#define FATALUDPREADERR(errno) (((errno = WSAGetLastError()) != WSAEWOULDBLOCK))

// After:
#define FATALUDPREADERR(errno) (((errno = WSAGetLastError()) != WSAEWOULDBLOCK) && (errno != WSAECONNRESET))
```

This ensures that even if `SIO_UDP_CONNRESET` is not applied (e.g., older Windows SDK), the server thread survives the transient error.

## Impact

- **Without fix:** Windows UDP servers are effectively single-use — they must be restarted between every client connection
- **With fix:** Windows UDP servers can serve multiple consecutive clients, matching Linux/macOS behavior
- **Risk:** None — `SIO_UDP_CONNRESET` only suppresses spurious ICMP-triggered errors that are silently ignored on all other platforms

## Resolution (2026-03-24)

Patch implemented and verified against iperf2 2.2.1 on Windows 10/11 (iperf2 2.2.1, macOS client via Tailscale VPN).

**Test: 3 consecutive UDP clients against a persistent daemon (`-s -u -D`)**

| Run | Before patch | After patch |
|---|---|---|
| 1 | Server Report ✓ | Server Report ✓ |
| 2 | WARNING — socket poisoned ✗ | Server Report ✓ |
| 3 | WARNING ✗ | Server Report ✓ |

Server process remained alive throughout all 3 runs (confirmed via `tasklist`).

**Patch and binary:**
- Patch file (against 2.2.1 tarball): https://github.com/DipodDP/iperf2/blob/master/wsaeconnreset.patch
- Fork with patch applied: https://github.com/DipodDP/iperf2
- Pre-built Windows x64 binary: https://github.com/DipodDP/iperf2/releases/tag/v2.2.1-wsaeconnreset1

The patch applies cleanly with `patch -p1` from the iperf2 2.2.1 source root.

## References

- [Microsoft KB 263823 — SIO_UDP_CONNRESET](https://learn.microsoft.com/en-us/windows/win32/winsock/winsock-ioctls)
- [Go issue #5834 — original SIO_UDP_CONNRESET fix](https://github.com/golang/go/issues/5834)
- [Go issue #68614 — SIO_UDP_NETRESET addition](https://github.com/golang/go/issues/68614)
- [tokio #2017 — Unexpected WSAECONNRESET from UDP recv_from](https://github.com/tokio-rs/tokio/issues/2017)
- [pjsip #1197 — WSAECONNRESET stops UDP transport](https://github.com/pjsip/pjproject/issues/1197)
- [Flutter #155823 — Windows UDP bug](https://github.com/flutter/flutter/issues/155823)
