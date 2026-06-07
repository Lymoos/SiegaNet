// Command sieganet-ctl manages peers on the server. It reads the server config
// for the peer-store path, inner subnet, domain and magic path, then operates on
// the store entirely through the peers.Store interface.
//
//	sieganet-ctl -config server.toml add <name>     # create peer, print config + QR
//	sieganet-ctl -config server.toml list
//	sieganet-ctl -config server.toml remove <name>
//	sieganet-ctl -config server.toml revoke <name>
//	sieganet-ctl -config server.toml rotate-psk <name>   # print new config + QR
package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"net/netip"
	"os"
	"text/tabwriter"
	"time"

	"github.com/mdp/qrterminal/v3"

	"github.com/lymoos/sieganet/internal/config"
	"github.com/lymoos/sieganet/internal/peers"
)

func main() {
	cfgPath := flag.String("config", "configs/server.toml", "path to server config (TOML)")
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		usage()
	}

	cfg, err := config.LoadServer(*cfgPath)
	if err != nil {
		fatal(err)
	}
	store, server, err := openStore(cfg)
	if err != nil {
		fatal(err)
	}

	cmd := args[0]
	switch cmd {
	case "add":
		needName(args)
		p, err := store.Add(args[1])
		if err != nil {
			fatal(err)
		}
		fmt.Printf("# added peer %q with inner IP %s\n\n", p.ID, p.InnerIP)
		printClient(cfg, server, p)
	case "rotate-psk":
		needName(args)
		p, err := store.RotatePSK(args[1])
		if err != nil {
			fatal(err)
		}
		fmt.Printf("# rotated PSK for %q (re-import this config)\n\n", p.ID)
		printClient(cfg, server, p)
	case "remove":
		needName(args)
		if err := store.Remove(args[1]); err != nil {
			fatal(err)
		}
		fmt.Printf("removed peer %q\n", args[1])
	case "revoke":
		needName(args)
		if err := store.Revoke(args[1]); err != nil {
			fatal(err)
		}
		fmt.Printf("revoked peer %q (authentication now refused; record kept)\n", args[1])
	case "list":
		list(store)
	default:
		usage()
	}
}

func openStore(cfg *config.Server) (peers.Store, string, error) {
	subnet, err := netip.ParsePrefix(cfg.InnerSubnet)
	if err != nil {
		return nil, "", fmt.Errorf("inner_subnet: %w", err)
	}
	serverIP, err := netip.ParseAddr(cfg.ServerInnerIP)
	if err != nil {
		return nil, "", fmt.Errorf("server_inner_ip: %w", err)
	}
	store, err := peers.NewFileStore(cfg.PeersStore, subnet, serverIP)
	if err != nil {
		return nil, "", err
	}
	if cfg.Domain == "" {
		return nil, "", errors.New("config: domain is required to print client configs")
	}
	server := net.JoinHostPort(cfg.Domain, portOf(cfg.ListenUDP))
	return store, server, nil
}

func printClient(cfg *config.Server, server string, p peers.Peer) {
	if cfg.TunnelPath == "" {
		fmt.Fprintln(os.Stderr, "warning: tunnel_path is empty in the server config")
	}
	cc := peers.NewClientConfig(p, server, cfg.TunnelPath, cfg.DNS, 1280)
	fmt.Println("---- client.toml ----")
	fmt.Print(cc.TOML())
	fmt.Println("---------------------")
	fmt.Println("Scan to import on Android:")
	qrterminal.GenerateHalfBlock(cc.URL(), qrterminal.L, os.Stdout)
}

func list(store peers.Store) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PEER\tINNER IP\tREVOKED\tACTIVE\tCREATED")
	for _, p := range store.List() {
		fmt.Fprintf(w, "%s\t%s\t%t\t%d\t%s\n",
			p.ID, p.InnerIP, p.Revoked, store.ActiveSessions(p.ID), p.Created.Format(time.RFC3339))
	}
	w.Flush()
}

func portOf(listen string) string {
	if _, port, err := net.SplitHostPort(listen); err == nil && port != "" {
		return port
	}
	return "443"
}

func needName(args []string) {
	if len(args) < 2 {
		fatal(fmt.Errorf("%s requires a peer name", args[0]))
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: sieganet-ctl -config server.toml <add|remove|revoke|rotate-psk|list> [name]")
	os.Exit(2)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "sieganet-ctl:", err)
	os.Exit(1)
}
