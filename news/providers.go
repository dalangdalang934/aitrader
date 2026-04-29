package news

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"aitrade/mcp"
)

// Provider 定义可插拔新闻来源。
type Provider interface {
	Name() string
	Run(ctx context.Context, svc *Service)
}

// WebSearchOptions 控制基于搜索模型的新闻补源。
type WebSearchOptions struct {
	Enabled  bool
	Query    string
	Interval time.Duration
	Client   *mcp.Client
	Provider string
	Model    string
}

type BWERSSProvider struct {
	URL string
}

func (p BWERSSProvider) Name() string {
	return "bwe-rss"
}

func (p BWERSSProvider) Run(ctx context.Context, svc *Service) {
	if strings.TrimSpace(p.URL) == "" {
		return
	}
	svc.runHTTP(ctx, strings.TrimSpace(p.URL))
}

type BWEWebSocketProvider struct {
	URL string
}

func (p BWEWebSocketProvider) Name() string {
	return "bwe-websocket"
}

func (p BWEWebSocketProvider) Run(ctx context.Context, svc *Service) {
	target := strings.TrimSpace(p.URL)
	if target == "" {
		return
	}
	switch {
	case strings.HasPrefix(target, "http://"), strings.HasPrefix(target, "https://"):
		svc.runHTTP(ctx, target)
	default:
		svc.runWebsocket(ctx, target)
	}
}



func buildProviders(opts Options) []Provider {
	providers := make([]Provider, 0, 4)
	if rss := strings.TrimSpace(opts.RSSURL); rss != "" {
		providers = append(providers, BWERSSProvider{URL: rss})
	}
	if ws := strings.TrimSpace(opts.WebsocketURL); ws != "" && ws != opts.RSSURL {
		providers = append(providers, BWEWebSocketProvider{URL: ws})
	}
	if opts.WebSearch.Enabled && opts.WebSearch.Client != nil {
		providers = append(providers, NewWebSearchProvider(opts.WebSearch))
	}
	if opts.OpenNews.Enabled {
		providers = append(providers, OpenNewsProvider{
			APIURL: opts.OpenNews.APIURL,
			WSURL:  opts.OpenNews.WSURL,
			APIKey: opts.OpenNews.APIKey,
		})
	}
	return providers
}

func (s *Service) runProviders(ctx context.Context) {
	if len(s.providers) == 0 {
		log.Printf("%s: 未启用任何新闻 provider", s.loggerPrefix)
		return
	}

	var wg sync.WaitGroup
	for _, provider := range s.providers {
		provider := provider
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Printf("%s: 启动 provider %s", s.loggerPrefix, provider.Name())
			provider.Run(ctx, s)
		}()
	}

	<-ctx.Done()
	wg.Wait()
}
