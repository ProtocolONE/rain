package resolver

import (
	"context"
	"errors"
	"net"
	"strconv"
	"time"

	"github.com/ProtocolONE/rain/internal/blocklist"
)

var (
	errBlocked        = errors.New("ip is blocked")
	errNotIPv4Address = errors.New("not ipv4 address")
)

func Resolve(ctx context.Context, hostport string, timeout time.Duration, bl *blocklist.Blocklist) (net.IP, int, error) {
	host, portStr, err := net.SplitHostPort(hostport)
	if err != nil {
		return nil, 0, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, 0, err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		ip, err = ResolveIPv4(ctx, timeout, host)
		if err != nil {
			return nil, 0, err
		}
	}
	i4 := ip.To4()
	if i4 == nil {
		return nil, 0, errNotIPv4Address
	}
	if bl != nil && bl.Blocked(ip) {
		return nil, 0, errBlocked
	}
	return i4, port, nil
}

func ResolveIPv4(ctx context.Context, timeout time.Duration, host string) (net.IP, error) {
	var cancel func()
	ctx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, ia := range addrs {
		i4 := ia.IP.To4()
		if i4 != nil {
			return i4, nil
		}
	}
	return nil, errNotIPv4Address
}
