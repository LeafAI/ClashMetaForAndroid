package tunnel

import (
	"context"
	"sync"
	"time"

	"github.com/metacubex/mihomo/adapter/outboundgroup"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/constant/provider"
	"github.com/metacubex/mihomo/log"
	"github.com/metacubex/mihomo/tunnel"
)

func HealthCheck(name string) {
	p := tunnel.Proxies()[name]

	if p == nil {
		log.Warnln("Request health check for `%s`: not found", name)

		return
	}

	g, ok := p.Adapter().(outboundgroup.ProxyGroup)
	if !ok {
		log.Warnln("Request health check for `%s`: invalid type %s", name, p.Type().String())

		return
	}

	proxies := g.Proxies()
	names := proxyNames(proxies)

	delays.BeginTest(names)
	defer delays.EndTest(names)

	if name == globalGroupName {
		testGroupProxies(proxies)
		recordDelays(proxies)

		return
	}

	wg := &sync.WaitGroup{}

	for _, pr := range g.Providers() {
		wg.Add(1)

		go func(provider provider.ProxyProvider) {
			provider.HealthCheck()

			wg.Done()
		}(pr)
	}

	wg.Wait()

	recordDelays(proxies)
}

const (
	globalGroupName    = "GLOBAL"
	globalTestTimeout  = 5 * time.Second
	globalTestParallel = 10
)

// testGroupProxies runs a delay test on every proxy itself. The implicit GLOBAL
// group is backed by a provider created without a test URL, which mihomo's
// provider health check silently skips, so it has to be tested directly.
func testGroupProxies(proxies []C.Proxy) {
	wg := &sync.WaitGroup{}
	slots := make(chan struct{}, globalTestParallel)

	for _, p := range proxies {
		wg.Add(1)
		slots <- struct{}{}

		go func(p C.Proxy) {
			defer wg.Done()
			defer func() { <-slots }()

			ctx, cancel := context.WithTimeout(context.Background(), globalTestTimeout)
			defer cancel()

			_, _ = p.URLTest(ctx, delayTestURL(p), nil)
		}(p)
	}

	wg.Wait()
}

func HealthCheckAll() {
	mode := tunnel.Mode()

	if mode == tunnel.Direct {
		return
	}

	for _, g := range QueryProxyGroupNames(false) {
		if g == globalGroupName && mode != tunnel.Global {
			continue
		}

		go func(group string) {
			HealthCheck(group)
		}(g)
	}
}
