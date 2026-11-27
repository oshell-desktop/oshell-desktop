//go:build darwin || linux

package core

import (
	"context"
	"net"
)

func listenMCP(root string) (net.Listener, func(), error) {
	return listenLaunch(mcpEndpoint(root))
}

func dialMCP(ctx context.Context, root string) (net.Conn, error) {
	return dialLaunch(ctx, mcpEndpoint(root))
}
