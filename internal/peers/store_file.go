package peers

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
)

// FileStore is a TOML-backed Store. Inner IPs are persisted, so a peer's address
// is stable across restarts and is never reassigned while the peer exists.
type FileStore struct {
	path  string
	alloc allocator

	mu       sync.RWMutex
	peers    map[string]*Peer
	used     map[netip.Addr]bool
	sessions map[string]int // runtime session counts, not persisted
}

// NewFileStore opens (or creates) a peer store at path, allocating inner IPs from
// subnet and reserving serverIP.
func NewFileStore(path string, subnet netip.Prefix, serverIP netip.Addr) (*FileStore, error) {
	if !subnet.IsValid() || !serverIP.IsValid() {
		return nil, fmt.Errorf("peers: invalid subnet/server IP")
	}
	s := &FileStore{
		path:     path,
		alloc:    allocator{subnet: subnet.Masked(), serverIP: serverIP},
		peers:    map[string]*Peer{},
		used:     map[netip.Addr]bool{},
		sessions: map[string]int{},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

type peerDTO struct {
	ID      string    `toml:"id"`
	PSK     string    `toml:"psk"`
	InnerIP string    `toml:"inner_ip"`
	Revoked bool      `toml:"revoked"`
	Created time.Time `toml:"created"`
}

type fileModel struct {
	Peer []peerDTO `toml:"peer"`
}

func (s *FileStore) load() error {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil // empty store
	}
	if err != nil {
		return fmt.Errorf("peers: read %s: %w", s.path, err)
	}
	var fm fileModel
	if err := toml.Unmarshal(data, &fm); err != nil {
		return fmt.Errorf("peers: parse %s: %w", s.path, err)
	}
	for _, d := range fm.Peer {
		psk, err := base64.StdEncoding.DecodeString(d.PSK)
		if err != nil {
			return fmt.Errorf("peers: peer %q bad psk: %w", d.ID, err)
		}
		ip, err := netip.ParseAddr(d.InnerIP)
		if err != nil {
			return fmt.Errorf("peers: peer %q bad inner_ip: %w", d.ID, err)
		}
		s.peers[d.ID] = &Peer{ID: d.ID, PSK: psk, InnerIP: ip, Revoked: d.Revoked, Created: d.Created}
		s.used[ip] = true
	}
	return nil
}

// saveLocked writes the store atomically. The caller must hold s.mu.
func (s *FileStore) saveLocked() error {
	fm := fileModel{}
	for _, p := range s.peers {
		fm.Peer = append(fm.Peer, peerDTO{
			ID:      p.ID,
			PSK:     base64.StdEncoding.EncodeToString(p.PSK),
			InnerIP: p.InnerIP.String(),
			Revoked: p.Revoked,
			Created: p.Created,
		})
	}
	sort.Slice(fm.Peer, func(i, j int) bool { return fm.Peer[i].InnerIP < fm.Peer[j].InnerIP })

	if dir := filepath.Dir(s.path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err := toml.NewEncoder(f).Encode(fm); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func newPSK() ([]byte, error) {
	k := make([]byte, pskLen)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	return k, nil
}

func (s *FileStore) Lookup(peerID string) (Peer, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.peers[peerID]
	if !ok {
		return Peer{}, false
	}
	return p.clone(), true
}

func (s *FileStore) Add(peerID string) (Peer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.peers[peerID]; ok {
		return Peer{}, ErrExists
	}
	ip, err := s.alloc.next(s.used)
	if err != nil {
		return Peer{}, err
	}
	psk, err := newPSK()
	if err != nil {
		return Peer{}, err
	}
	p := &Peer{ID: peerID, PSK: psk, InnerIP: ip, Created: time.Now().UTC()}
	s.peers[peerID] = p
	s.used[ip] = true
	if err := s.saveLocked(); err != nil {
		delete(s.peers, peerID)
		delete(s.used, ip)
		return Peer{}, err
	}
	return p.clone(), nil
}

func (s *FileStore) Remove(peerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.peers[peerID]
	if !ok {
		return ErrNotFound
	}
	delete(s.peers, peerID)
	delete(s.used, p.InnerIP)
	delete(s.sessions, peerID)
	return s.saveLocked()
}

func (s *FileStore) List() []Peer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Peer, 0, len(s.peers))
	for _, p := range s.peers {
		out = append(out, p.clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].InnerIP.Less(out[j].InnerIP) })
	return out
}

func (s *FileStore) Revoke(peerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.peers[peerID]
	if !ok {
		return ErrNotFound
	}
	p.Revoked = true
	return s.saveLocked()
}

func (s *FileStore) RotatePSK(peerID string) (Peer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.peers[peerID]
	if !ok {
		return Peer{}, ErrNotFound
	}
	psk, err := newPSK()
	if err != nil {
		return Peer{}, err
	}
	p.PSK = psk
	p.Revoked = false
	if err := s.saveLocked(); err != nil {
		return Peer{}, err
	}
	return p.clone(), nil
}

func (s *FileStore) PSK(peerID string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.peers[peerID]
	if !ok || p.Revoked {
		return nil, false
	}
	return append([]byte(nil), p.PSK...), true
}

func (s *FileStore) SessionOpened(peerID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[peerID]++
	return s.sessions[peerID]
}

func (s *FileStore) SessionClosed(peerID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions[peerID] > 0 {
		s.sessions[peerID]--
	}
	return s.sessions[peerID]
}

func (s *FileStore) ActiveSessions(peerID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[peerID]
}

// compile-time assertion that FileStore implements the interface.
var _ Store = (*FileStore)(nil)
