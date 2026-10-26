// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"net"
	"strings"
	"sync"
)

// A failed listener Close is not evidence that its bound port was released.
// Keep ownership until shutdown completes and every listener is confirmed closed.
func (t *transport) releaseDrainedTunnels() {
	t.mu.Lock()
	drained := t.shutdownComplete
	for closer := range t.closers {
		if _, listener := closer.(net.Listener); listener {
			drained = false
			break
		}
	}
	t.mu.Unlock()
	if drained {
		t.releaseTunnels()
	}
}

var tunnelOwnership = struct {
	sync.Mutex
	owners map[string]*transport
}{owners: map[string]*transport{}}

func hasEnabledTunnel(tunnels []TunnelRule) bool {
	for _, t := range tunnels {
		if t.Enabled {
			return true
		}
	}
	return false
}
func (t *transport) claimTunnels(id string) bool {
	key := strings.ToUpper(id)
	tunnelOwnership.Lock()
	defer tunnelOwnership.Unlock()
	if owner := tunnelOwnership.owners[key]; owner != nil && owner != t {
		return false
	}
	tunnelOwnership.owners[key] = t
	t.tunnelProfile = key
	return true
}
func (t *transport) releaseTunnels() {
	tunnelOwnership.Lock()
	defer tunnelOwnership.Unlock()
	if t.tunnelProfile != "" && tunnelOwnership.owners[t.tunnelProfile] == t {
		delete(tunnelOwnership.owners, t.tunnelProfile)
	}
}

// TunnelPolicy describes the initial transport decision, also shared by clones.
func (c *Connection) TunnelPolicy() string {
	if c == nil || c.transport == nil {
		return "unknown"
	}
	if c.transport.tunnelPolicy == "" {
		return "unknown"
	}
	return c.transport.tunnelPolicy
}
