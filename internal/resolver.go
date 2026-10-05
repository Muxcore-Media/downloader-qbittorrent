package internal

import (
	"context"
	"net"
	"net/netip"
)

type defaultResolver struct{}

func (defaultResolver) LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, network, host)
}
