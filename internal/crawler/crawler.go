package crawler

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"

	genmodels "opensearch/gen/go/models"
	"opensearch/internal/merger"
)

const maxCrawlURLs = 3

var sufficiencyThresholds = map[string]float64{
	"factual": 0.70,
	"code": 0.65,
	"general": 0.65,
	"news": 0.60,
	"commercial": 0.60,
	"research": 0.45,
}

func Decide(ctx context.Context, req Request, modelConn *grpc.ClientConn, spiderConn *grpc.ClientConn) Decision {
	if len(req.Results) == 0 {
		return Decision{Sufficient: true}
	}

	threshold := thresholdFor(req.Intent)
	top := topResults(req)
	avg := averageSufficiency(ctx, req.Query, top, modelConn)

	if avg >= threshold {
		slog.Debug("snippets sufficient", "intent", req.Intent, "score", avg, "threshold", threshold)
		return Decision{Sufficient: true}
	}

	slog.Info("snippets insufficient, invoking spider", "intent", req.Intent, "score", avg, "threshold", threshold)

	urls := make([]string, len(top))
	for i, r := range top {
		urls[i] = r.URL
	}

	spider := newSpiderClient(spiderConn)
	enriched, err := spider.fetch(ctx, urls, top)
	if err != nil {
		slog.Error("spider fetch failed, falling back to snippets", "error", err)
		return Decision{Sufficient: true}
	}

	return Decision{
		Sufficient: false,
		URLs: urls,
		EnrichedResults: enriched,
	}
}

func averageSufficiency(ctx context.Context, query string, results []merger.Result, modelConn *grpc.ClientConn) float64 {
	client := genmodels.NewModelServiceClient(modelConn)

	var total float64
	for _, r := range results {
		resp, err := client.Relevance(ctx, &genmodels.RelevanceRequest{
			Query:   query,
			Snippet: r.Snippet,
		})
		if err != nil {
			slog.Warn("relevance call failed, using zero score", "url", r.URL, "error", err)
			continue
		}

		sufficiency := (float64(resp.Score) * 0.50) +
			(snippetDensity(r.Snippet) * 0.30) +
			(sourceAuthority(r.Domain) * 0.20)

		total += sufficiency
	}

	if len(results) == 0 {
		return 0
	}
	return total / float64(len(results))
}

func snippetDensity(snippet string) float64 {
	length := len(snippet)
	switch {
	case length >= 300:
		return 1.0
	case length >= 150:
		return 0.7
	case length >= 50:
		return 0.4
	default:
		return 0.1
	}
}

func sourceAuthority(domain string) float64 {
	authoritative := map[string]bool{
		"wikipedia.org": true,
		"github.com": true,
		"stackoverflow.com": true,
		"docs.python.org": true,
		"developer.mozilla.org": true,
		"pkg.go.dev": true,
		"arxiv.org": true,
	}
	if authoritative[domain] {
		return 1.0
	}
	return 0.5
}

func thresholdFor(intent string) float64 {
	if t, ok := sufficiencyThresholds[intent]; ok {
		return t
	}
	return 0.65
}

func topResults(req Request) []merger.Result {
	n := maxCrawlURLs
	if len(req.Results) < n {
		n = len(req.Results)
	}
	return req.Results[:n]
}

func topURLs(req Request) []string {
	top := topResults(req)
	urls := make([]string, len(top))
	for i, r := range top {
		urls[i] = r.URL
	}
	return urls
}
func (d *Decider) Decide(ctx context.Context, req Request) Decision {
	return Decide(ctx, req, d.ModelConn, d.SpiderConn)
}