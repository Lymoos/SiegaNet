# SiegaNet — Windows verification checklist (Phase 2)

Everything in the Windows client (wintun, IP Helper routes, NRPT DNS, the WFP
kill-switch) is **unverifiable in CI** — it only runs against the real Windows
network stack. This document is the single, ordered checklist the operator runs
by hand on a Windows VM and/or PC to clear the accumulated "Windows debt" and
sign off Phase 2.

For every step: the exact **command**, the **expected** result, and what a
**failure** looks like (and the likely culprit). Run an elevated PowerShell
unless a step says otherwise.

Likely break points (per the code reports): the *semantics* of the IP Helper /
WFP calls (the struct *layouts* are already asserted at build time, but whether
the live API accepts them is not), and the real behaviour of NRPT.

---

## 0. Setup

### 0.1 Build the client (on the dev box)

```powershell
# amd64 (use GOARCH=arm64 for ARM devices)
$env:GOOS="windows"; $env:GOARCH="amd64"
go build -o sieganet-client.exe ./cmd/sieganet-client
```

The `requireAdministrator` manifest is already linked via the committed
`rsrc_windows_*.syso`. Confirm it is in the binary:

```powershell
Select-String -Path .\sieganet-client.exe -Pattern "requireAdministrator" -Encoding ascii | Select-Object -First 1
```
* **Expected:** one match.
* **Failure:** no match → the `.syso` was not linked (wrong filename/arch); the
  exe will not auto-elevate.

### 0.2 wintun.dll next to the exe

Download the **matching-arch** `wintun.dll` from <https://www.wintun.net> and put
it beside `sieganet-client.exe`.

### 0.3 A reachable server + client.toml

Use your VPS (Let's Encrypt cert → nothing extra) or a Linux box running
`sieganet-server` with `cert_mode=file` (then copy the test CA to the Windows box
and pass `-ca ca.crt`). Create the peer and config on the server:

```sh
sieganet-ctl -config server.toml add winbox      # prints client.toml + QR
```

Put that `client.toml` (with `kill_switch = true`) next to the exe. Note the
server's **public IP** — call it `<SERVER_IP>` below.

### 0.4 Launch

```powershell
.\sieganet-client.exe -config .\client.toml        # add -ca .\ca.crt for a private CA
```
Leave it running in one window; run the checks in another (elevated) window.

---

## 1. Adapter & wintun.dll

### 1.1 The wintun adapter comes up with the fixed GUID

```powershell
Get-NetAdapter | Where-Object InterfaceDescription -like "Wintun*" |
  Format-List Name, InterfaceDescription, InterfaceGuid, Status
```
* **Expected:** one adapter, `Status = Up`, `InterfaceGuid =
  {5C1E6A4E-9B27-4F3A-A170-53696567614E}` (the fixed SiegaNet GUID).
* **Failure:** no adapter → `Open`/CreateTUN failed (see the client log). Wrong
  GUID → not our adapter / GUID not honoured.

### 1.2 Missing-DLL error path (no panic)

Temporarily move the DLL and start the client:

```powershell
Rename-Item .\wintun.dll .\wintun.dll.bak
.\sieganet-client.exe -config .\client.toml
Rename-Item .\wintun.dll.bak .\wintun.dll
```
* **Expected:** a clear line like `wintun.dll not found next to the executable
  (expected ...); download the amd64 wintun.dll ...` and a clean exit.
* **Failure:** a Go panic / stack trace, or a silent hang → the pre-validation or
  `recover()` didn't catch it.

### 1.3 Wrong-architecture DLL error path

Put an **arm64** `wintun.dll` next to the **amd64** exe (or vice-versa) and start:

* **Expected:** `wintun.dll at ...: wrong architecture: image is arm64, this
  build needs amd64`.
* **Failure:** panic, or it proceeds and then crashes in the driver.

Restore the correct DLL before continuing.

---

## 2. Route interception (full tunnel + anti-loop)

### 2.1 The two /1 halves are on the tunnel; the endpoint /32 is on the physical NIC

```powershell
# Replace <SERVER_IP> with the server's public IP first.
Get-NetRoute -AddressFamily IPv4 |
  Where-Object { $_.DestinationPrefix -in '0.0.0.0/1','128.0.0.0/1' -or
                 $_.DestinationPrefix -eq '<SERVER_IP>/32' } |
  Format-Table DestinationPrefix, NextHop, InterfaceAlias, RouteMetric -Auto
```
* **Expected:**
  * `0.0.0.0/1` and `128.0.0.0/1` → `InterfaceAlias` = the **wintun** adapter;
  * `<SERVER_IP>/32` → `InterfaceAlias` = the **physical** NIC (Wi‑Fi/Ethernet),
    `NextHop` = your LAN gateway.
* **Failure:** the /1 routes missing or on the wrong interface →
  `CreateIpForwardEntry2` rejected the row (IP Helper *semantics*, likely culprit).
  The `/32` on the tunnel instead of the NIC → **anti-loop broken**, the QUIC
  connection would loop; the client usually can't connect at all.

### 2.2 Traffic actually goes through the tunnel

```powershell
tracert -d -h 3 1.1.1.1
(Invoke-WebRequest -UseBasicParsing https://api.ipify.org).Content
```
* **Expected:** first hop `10.7.0.1` (the server inner IP); the printed IP is your
  **server's** public IP.
* **Failure:** first hop is your LAN gateway / the IP is your home IP → traffic is
  not being captured (routes not effective).

---

## 3. DNS — no leak

### 3.1 The NRPT catch-all rule is installed

```powershell
Get-DnsClientNrptPolicy | Format-List Namespace, NameServers
```
* **Expected:** a rule with `Namespace = .` and `NameServers =` your tunnel DNS
  (e.g. `10.7.0.1` or the `dns` from the config).
* **Failure:** no `.` rule → NRPT write failed (registry path/format) — a likely
  break point.

### 3.2 Resolution uses the tunnel DNS, with no plaintext :53 on the physical link

Capture port 53 on the **physical** adapter with the built-in pktmon while you
resolve a name:

```powershell
pktmon filter remove
pktmon filter add DNS -p 53
$ifx = (Get-NetAdapter | ? InterfaceDescription -notlike "Wintun*" | ? Status -eq Up | Select -First 1).ifIndex
pktmon start --capture --comp $ifx -f C:\dnsleak.etl
Resolve-DnsName -Name example.com -Type A | Out-Null
Resolve-DnsName -Name $(New-Guid).Guid".example" -Type A -ErrorAction SilentlyContinue | Out-Null
pktmon stop
pktmon format C:\dnsleak.etl -o C:\dnsleak.txt
Select-String C:\dnsleak.txt -Pattern "UDP" | Select-String "\.53"
```
* **Expected:** **no** matching lines (no plaintext UDP/53 left the physical NIC —
  the queries went inside the QUIC tunnel).
* **Failure:** any UDP/53 packet on the physical adapter → **DNS leak**. Check the
  NRPT rule (3.1) and the kill-switch (step 5, which also blocks stray :53).

### 3.3 External confirmation

Open <https://www.dnsleaktest.com> → **Standard test**.
* **Expected:** only your server's resolver/network is listed.
* **Failure:** your ISP's resolver appears → leak.

---

## 4. IPv6 — fully dead (no dual-stack leak)

The tunnel is IPv4; IPv6 must be entirely blocked, or a dual-stack host leaks
around the IPv4 tunnel.

```powershell
Test-NetConnection -ComputerName 2606:4700:4700::1111 -Port 443 -InformationLevel Quiet
ping -6 -n 2 google.com
Resolve-DnsName -Type AAAA example.com -ErrorAction SilentlyContinue
try { (Invoke-WebRequest -UseBasicParsing -TimeoutSec 5 https://api6.ipify.org).Content } catch { "IPv6 blocked: $($_.Exception.Message)" }
```
* **Expected:** `Test-NetConnection` → `False`; `ping -6` → fails; the IPv6-only
  fetch → times out / "IPv6 blocked".
* **Failure:** any IPv6 reachability → the WFP IPv6 block isn't effective (check
  the v6 block filters; see step 5.3). Also run <https://test-ipv6.com> →
  expected: "no IPv6 address" / IPv6 not reachable.

---

## 5. Kill-switch — no leak window

### 5.1 The WFP filters exist

```powershell
netsh wfp show filters file=C:\wfp.xml | Out-Null
Select-String C:\wfp.xml -Pattern "SiegaNet" | Select-Object -First 5
```
* **Expected:** several matches (the SiegaNet provider/sublayer + filters).
* **Failure:** none → `FwpmFilterAdd0`/engine open failed (WFP *call semantics* —
  a likely break point; the client log will show `wfp ...` errors).

### 5.2 Drop the endpoint while the client is running → everything else dies

Simulate the server going away **without** stopping the client, so the tunnel
drops while the kill-switch stays up:

```powershell
New-NetFirewallRule -DisplayName "siega-droptest" -Direction Outbound `
  -RemoteAddress <SERVER_IP> -Action Block | Out-Null
Start-Sleep 6
# The tunnel is now down; the kill-switch must block all non-tunnel traffic:
Test-NetConnection 1.1.1.1 -Port 443 -InformationLevel Quiet
Resolve-DnsName example.com -Type A -ErrorAction SilentlyContinue
# restore:
Remove-NetFirewallRule -DisplayName "siega-droptest"
Start-Sleep 8
Test-NetConnection 1.1.1.1 -Port 443 -InformationLevel Quiet
```
* **Expected:** while the rule is in place → `Test-NetConnection` = **False** and
  DNS fails (fail-closed, **no leak window**); after removing it → the client
  reconnects and `Test-NetConnection` = **True**.
* **Failure:** any non-tunnel connectivity while the endpoint is blocked → a leak
  hole in the kill-switch.

### 5.3 IPv6 block is part of the same set

In `C:\wfp.xml` (5.1) confirm there are block filters on the IPv6 ALE layers
(`FWPM_LAYER_ALE_AUTH_CONNECT_V6` / `..._RECV_ACCEPT_V6`). Combined with step 4
this proves IPv6 is dead by policy, not by luck.

---

## 6. Crash → internet returns → restart sweeps the leftovers

### 6.1 Hard-kill the client (simulate a crash)

```powershell
Stop-Process -Name sieganet-client -Force
Start-Sleep 3
Test-NetConnection 1.1.1.1 -Port 443 -InformationLevel Quiet
Get-NetAdapter | Where-Object InterfaceDescription -like "Wintun*"
```
* **Expected (fail-open on crash, by design):** connectivity returns (`True`) —
  the DYNAMIC WFP session was torn down by the kernel and the wintun adapter was
  reaped. No SiegaNet adapter remains.
* **Failure:** still no internet → the WFP session was not dynamic (filters
  stranded). Adapter still present → it didn't get reaped (the startup sweep in
  6.2 should still clear it).

### 6.2 The registry leftover, then the startup sweep

NRPT lives in the registry and is **not** reaped on a crash, so right after the
kill it lingers:

```powershell
Get-DnsClientNrptPolicy | Format-List Namespace, NameServers   # stale "." rule may still show
```
Now restart the client and re-check **before** it finishes connecting (the sweep
runs first):

```powershell
.\sieganet-client.exe -config .\client.toml
# in another window, immediately:
Get-DnsClientNrptPolicy | Format-List Namespace, NameServers
```
* **Expected:** the startup `Sweep()` removed the stale `.` rule / stale policy /
  stuck adapter; the only `.` rule present is the freshly-written one (or none yet
  if checked mid-sweep). DNS works again afterwards.
* **Failure:** duplicate/stale NRPT rules accumulate across restarts, or DNS stays
  broken → the sweep isn't clearing the fixed markers.

---

## 7. Elevation / UAC

* From **Explorer**, double-click `sieganet-client.exe`.
  * **Expected:** a UAC prompt appears (the manifest's `requireAdministrator`).
  * **Failure:** no prompt and an immediate access-denied / TUN-create failure →
    the manifest isn't taking effect (see 0.1).
* From a **non-elevated** PowerShell, run it.
  * **Expected:** Windows shows the UAC consent dialog before it runs.

---

## Sign-off

Phase 2 is complete once 1–7 pass on a real Windows machine. Record any step that
fails with its output; per the plan, fixes are made against those concrete
results (the most likely areas are IP Helper / WFP call semantics and NRPT
behaviour). Nothing else should be coded until this checklist has been run.
