package helper

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"time"
	"unicode/utf8"
)

var nonPublicPushNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
}

// ResolvePushEndpoint возвращает только публичные IP; отправитель подключается к ним без повторного DNS.
func ResolvePushEndpoint(ctx context.Context, raw string) ([]net.IPAddr, error) {
	invalid := errors.New("недопустимый адрес push-подписки")
	if raw == "" || utf8.RuneCountInString(raw) > 2048 {
		return nil, invalid
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return nil, invalid
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return nil, invalid
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, u.Hostname())
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, invalid
	}
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip.IP)
		addr = addr.Unmap()
		if !ok || ip.Zone != "" || !addr.IsGlobalUnicast() || addr.IsPrivate() {
			return nil, invalid
		}
		// IPv6 вне глобального диапазона включает локальные и трансляционные адреса.
		if addr.Is6() && !netip.MustParsePrefix("2000::/3").Contains(addr) {
			return nil, invalid
		}
		for _, prefix := range nonPublicPushNetworks {
			if prefix.Contains(addr) {
				return nil, invalid
			}
		}
	}
	return ips, nil
}
