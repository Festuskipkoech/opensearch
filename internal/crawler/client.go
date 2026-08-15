package crawler

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"google.golang.org/grpc"

	gencrawler "opensearch/gen/go/crawler"
	"opensearch/internal/merger"
)

type spiderClient struct {
	client gencrawler.CrawlerServiceClient
}

func newSpiderClient(conn *grpc.ClientConn) *spiderClient {
	return &spiderClient{client: gencrawler.NewCrawlerServiceClient(conn)}
}

// fetch calls StreamCrawl on the Spider-rs service and reads results off the
// stream as each URL completes. Results arrive in completion order, not
// request order. The channel stays open until the stream closes or the
// context is cancelled.
func (s *spiderClient) fetch(ctx context.Context, urls []string, results []merger.Result) ([]merger.Result, error) {
	stream, err := s.client.StreamCrawl(ctx, &gencrawler.CrawlRequest{Urls: urls})
	if err != nil {
		return nil, fmt.Errorf("StreamCrawl: %w", err)
	}

	index := make(map[string]int, len(results))
	for i, r := range results {
		index[r.URL] = i
	}

	enriched := make([]merger.Result, len(results))
	copy(enriched, results)

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("stream recv: %w", err)
		}
		if msg.Error != "" {
			slog.Warn("spider fetch failed", "url", msg.Url, "error", msg.Error)
			continue
		}
		if i, ok := index[msg.Url]; ok {
			enriched[i].Content = msg.Content
			enriched[i].Tokens = int(msg.TokenCount)
		}
	}

	return enriched, nil
}