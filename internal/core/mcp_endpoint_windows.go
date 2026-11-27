package core

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Windows retains a named pipe while any client handle is open, even after
// the server closes. Publish a new private endpoint for each server lifetime
// so an idle bridge cannot prevent restarting MCP. The lifetime lock prevents
// another server from replacing a live endpoint. Peer SID and token checks
// remain mandatory; this local discovery file contains no credentials.
func listenMCP(root string) (net.Listener, func(), error) {
	endpoint := mcpEndpoint(root)
	dir := filepath.Dir(endpoint)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	unlock, err := lockNamedDirectory(ctx, dir, ".mcp-server.lock")
	if err != nil {
		return nil, nil, err
	}
	id := NewID()
	listener, _, err := listenLaunch(endpoint + "-" + id)
	if err != nil {
		unlock()
		return nil, nil, err
	}
	if err = AtomicWrite(endpoint+".endpoint", []byte(id)); err != nil {
		listener.Close()
		unlock()
		return nil, nil, err
	}
	return listener, func() {
		// Remove discovery while holding the lock, before a replacement starts.
		_ = os.Remove(endpoint + ".endpoint")
		unlock()
	}, nil
}

func dialMCP(ctx context.Context, root string) (net.Conn, error) {
	endpoint := mcpEndpoint(root)
	data, err := readAgentFile(endpoint+".endpoint", 36)
	if os.IsNotExist(err) {
		// New bridges can still connect to an older running Desktop.
		return dialLaunch(ctx, endpoint)
	}
	if err != nil {
		return nil, err
	}
	if len(data) != 36 || !uuidText.Match(data) {
		return nil, fmt.Errorf("MCP 本机通道标识无效")
	}
	return dialLaunch(ctx, endpoint+"-"+string(data))
}
