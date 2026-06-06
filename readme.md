root@DESKTOP-E2RTGAK:~/SiegaNet# sudo bash scripts/phase0-netns-test.sh
>> raising UDP socket buffer limits (sysctl)
>> building binaries (-tags phase0_insecure)
go: downloading go1.25.0 (linux/amd64)
go: downloading golang.zx2c4.com/wireguard v0.0.0-20260522210424-ecfc5a8d5446
go: downloading github.com/quic-go/quic-go v0.60.0
go: downloading golang.org/x/sys v0.45.0
go: downloading golang.org/x/net v0.55.0
go: downloading golang.org/x/crypto v0.51.0
>> creating namespaces and veth

############################################################
## PASS: clean link
############################################################
>> ping 10.7.0.1 through the tunnel
PING 10.7.0.1 (10.7.0.1) 56(84) bytes of data.
64 bytes from 10.7.0.1: icmp_seq=1 ttl=64 time=0.550 ms
64 bytes from 10.7.0.1: icmp_seq=2 ttl=64 time=1.14 ms
64 bytes from 10.7.0.1: icmp_seq=3 ttl=64 time=0.748 ms
64 bytes from 10.7.0.1: icmp_seq=4 ttl=64 time=0.825 ms
64 bytes from 10.7.0.1: icmp_seq=5 ttl=64 time=0.654 ms

--- 10.7.0.1 ping statistics ---
5 packets transmitted, 5 received, 0% packet loss, time 4045ms
rtt min/avg/max/mdev = 0.550/0.783/1.138/0.199 ms

>> PMTU: ping -M do -s 1400 (1428B, DF) must NOT vanish silently
PING 10.7.0.1 (10.7.0.1) 1400(1428) bytes of data.
ping: local error: message too long, mtu=1242
ping: local error: message too long, mtu=1242

--- 10.7.0.1 ping statistics ---
2 packets transmitted, 0 received, +2 errors, 100% packet loss, time 1020ms

   ^ either replies, or a clear 'message too long'/frag-needed — not silence

>> iperf3 through the tunnel
Connecting to host 10.7.0.1, port 5201
[  5] local 10.7.0.2 port 52820 connected to 10.7.0.1 port 5201
[ ID] Interval           Transfer     Bitrate         Retr  Cwnd
[  5]   0.00-1.00   sec   110 MBytes   923 Mbits/sec    0   3.27 MBytes
[  5]   1.00-2.00   sec   103 MBytes   867 Mbits/sec  235   2.44 MBytes
[  5]   2.00-3.00   sec   111 MBytes   928 Mbits/sec    7   2.40 MBytes
[  5]   3.00-4.00   sec   112 MBytes   940 Mbits/sec   24   1.37 MBytes
[  5]   4.00-5.00   sec   112 MBytes   942 Mbits/sec    0   1.45 MBytes
- - - - - - - - - - - - - - - - - - - - - - - - -
[ ID] Interval           Transfer     Bitrate         Retr
[  5]   0.00-5.00   sec   548 MBytes   920 Mbits/sec  266             sender
[  5]   0.00-5.01   sec   546 MBytes   914 Mbits/sec                  receiver

iperf Done.

>> MSS clamp rule (client OUTPUT mangle) — pkts column proves it matched the SYN
Chain OUTPUT (policy ACCEPT 493K packets, 1203M bytes)
    2   120 TCPMSS     6    --  *      siega0  0.0.0.0/0            0.0.0.0/0            tcp flags:0x06/0x02 TCPMSS set 1202

>> data-plane counters (clean link):
   [srv] [sieganet-server] 2026/06/06 20:08:49 udp socket buffers: rcvbuf=33554432 sndbuf=33554432
   [srv] [sieganet-server] 2026/06/06 20:08:51 mtu gate: max_datagram=1243 inner_mtu=1242 (mss<=1202)
   [srv] [sieganet-server] 2026/06/06 20:09:05 STATS tx_tun=178610 tx_sent=178610 tx_drop_oversize=0 tx_drop_err=0 rx_recv=482912 rx_written=482912 rx_drop_malformed=0 rx_drop_backlog=0 rx_drop_tun=0
   [srv] [sieganet-server] 2026/06/06 20:09:05 iface siega0 kernel drops: rx_dropped=0 tx_dropped=0
   [cli] [sieganet-client] 2026/06/06 20:08:51 udp socket buffers: rcvbuf=33554432 sndbuf=33554432
   [cli] [sieganet-client] 2026/06/06 20:08:51 mtu gate: max_datagram=1243 inner_mtu=1242 (mss<=1202)
   [cli] [sieganet-client] 2026/06/06 20:09:05 STATS tx_tun=483178 tx_sent=483178 tx_drop_oversize=0 tx_drop_err=0 rx_recv=178610 rx_written=178610 rx_drop_malformed=0 rx_drop_backlog=0 rx_drop_tun=0
   [cli] [sieganet-client] 2026/06/06 20:09:05 iface siega0 kernel drops: rx_dropped=0 tx_dropped=0
   delivery (client tx_sent=483178 -> server rx_recv=482912): 99% of upstream datagrams arrived
   (a gap here = datagrams lost on the path/buffers/quic-go rcv queue, NOT inner-TCP retransmits)
   server kernel UDP during iperf: InErrors+=0 RcvbufErrors+=0
   (RcvbufErrors==0 with a delivery gap means the kernel buffer was fine and the
    loss was quic-go's internal 128-deep datagram queue, not SO_RCVBUF — raise -rx-workers)

>> impairment via tc-netem (delay 60ms loss 2%, each direction)

############################################################
## PASS: tc-netem delay 60ms loss 2%
############################################################
>> ping 10.7.0.1 through the tunnel
PING 10.7.0.1 (10.7.0.1) 56(84) bytes of data.
64 bytes from 10.7.0.1: icmp_seq=1 ttl=64 time=122 ms
64 bytes from 10.7.0.1: icmp_seq=2 ttl=64 time=121 ms
64 bytes from 10.7.0.1: icmp_seq=3 ttl=64 time=121 ms
64 bytes from 10.7.0.1: icmp_seq=4 ttl=64 time=121 ms
64 bytes from 10.7.0.1: icmp_seq=5 ttl=64 time=121 ms

--- 10.7.0.1 ping statistics ---
5 packets transmitted, 5 received, 0% packet loss, time 4005ms
rtt min/avg/max/mdev = 121.163/121.426/122.003/0.305 ms

>> PMTU: ping -M do -s 1400 (1428B, DF) must NOT vanish silently
PING 10.7.0.1 (10.7.0.1) 1400(1428) bytes of data.
ping: local error: message too long, mtu=1242
ping: local error: message too long, mtu=1242

--- 10.7.0.1 ping statistics ---
2 packets transmitted, 0 received, +2 errors, 100% packet loss, time 1023ms

   ^ either replies, or a clear 'message too long'/frag-needed — not silence

>> iperf3 through the tunnel
Connecting to host 10.7.0.1, port 5201
[  5] local 10.7.0.2 port 40424 connected to 10.7.0.1 port 5201
[ ID] Interval           Transfer     Bitrate         Retr  Cwnd
[  5]   0.00-1.00   sec   128 KBytes  1.05 Mbits/sec    4   15.1 KBytes
[  5]   1.00-2.00   sec   128 KBytes  1.05 Mbits/sec    3   9.30 KBytes
[  5]   2.00-3.00   sec  0.00 Bytes  0.00 bits/sec    2   5.81 KBytes
[  5]   3.00-4.00   sec   128 KBytes  1.05 Mbits/sec    1   6.97 KBytes
[  5]   4.00-5.00   sec  0.00 Bytes  0.00 bits/sec    1   6.97 KBytes
- - - - - - - - - - - - - - - - - - - - - - - - -
[ ID] Interval           Transfer     Bitrate         Retr
[  5]   0.00-5.00   sec   384 KBytes   629 Kbits/sec   11             sender
[  5]   0.00-5.12   sec   256 KBytes   409 Kbits/sec                  receiver

iperf Done.

>> MSS clamp rule (client OUTPUT mangle) — pkts column proves it matched the SYN
Chain OUTPUT (policy ACCEPT 494K packets, 1204M bytes)
    3   180 TCPMSS     6    --  *      siega0  0.0.0.0/0            0.0.0.0/0            tcp flags:0x06/0x02 TCPMSS set 1202

>> data-plane counters (tc-netem delay 60ms loss 2%):
   [srv] [sieganet-server] 2026/06/06 20:09:06 udp socket buffers: rcvbuf=33554432 sndbuf=33554432
   [srv] [sieganet-server] 2026/06/06 20:09:08 mtu gate: max_datagram=1243 inner_mtu=1242 (mss<=1202)
   [srv] [sieganet-server] 2026/06/06 20:09:24 STATS tx_tun=270 tx_sent=270 tx_drop_oversize=0 tx_drop_err=0 rx_recv=343 rx_written=343 rx_drop_malformed=0 rx_drop_backlog=0 rx_drop_tun=0
   [srv] [sieganet-server] 2026/06/06 20:09:24 iface siega0 kernel drops: rx_dropped=0 tx_dropped=0
   [cli] [sieganet-client] 2026/06/06 20:09:08 udp socket buffers: rcvbuf=33554432 sndbuf=33554432
   [cli] [sieganet-client] 2026/06/06 20:09:08 mtu gate: max_datagram=1243 inner_mtu=1242 (mss<=1202)
   [cli] [sieganet-client] 2026/06/06 20:09:24 STATS tx_tun=356 tx_sent=356 tx_drop_oversize=0 tx_drop_err=0 rx_recv=262 rx_written=262 rx_drop_malformed=0 rx_drop_backlog=0 rx_drop_tun=0
   [cli] [sieganet-client] 2026/06/06 20:09:24 iface siega0 kernel drops: rx_dropped=0 tx_dropped=0
   delivery (client tx_sent=356 -> server rx_recv=343): 96% of upstream datagrams arrived
   (a gap here = datagrams lost on the path/buffers/quic-go rcv queue, NOT inner-TCP retransmits)
   server kernel UDP during iperf: InErrors+=0 RcvbufErrors+=0
   (RcvbufErrors==0 with a delivery gap means the kernel buffer was fine and the
    loss was quic-go's internal 128-deep datagram queue, not SO_RCVBUF — raise -rx-workers)

>> ALL PHASE 0 PASSES COMPLETED
root@DESKTOP-E2RTGAK:~/SiegaNet#
