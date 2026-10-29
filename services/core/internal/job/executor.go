package job

import (
	"context"
	"errors"
	"sync"

	"github.com/Decodo/pobear/pkg/fault"
	"github.com/Decodo/pobear/pkg/jobresult"
	"github.com/Decodo/pobear/pkg/jobspec"
	"github.com/Decodo/pobear/services/core/internal/job/pool"
	"github.com/Decodo/pobear/services/core/internal/job/workers"
)

type ScrapeWorker interface {
	Run(ctx context.Context, spec *jobspec.Spec, url, parentURL string, depth int, gate workers.EmitGate) (links []string, blockedReason string, kind fault.Kind, err error)
}

type CrawlWorker interface {
	Run(ctx context.Context, spec *jobspec.Spec) (Result, error)
}

type SinkCloser interface {
	Close() error
}

type Result = jobresult.Result

type Executor struct {
	scrape ScrapeWorker
	crawl  CrawlWorker
	sink   SinkCloser
}

func NewExecutor(scrape ScrapeWorker, crawl CrawlWorker, sink SinkCloser) *Executor {
	return &Executor{scrape: scrape, crawl: crawl, sink: sink}
}

func (e *Executor) Run(ctx context.Context, spec *jobspec.Spec) (result Result, err error) {
	defer func() {
		if closeErr := e.sink.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	if spec.Source.Crawl != nil {
		result, err = e.crawl.Run(ctx, spec)
		return
	}

	p := pool.New(spec.Limits.Concurrency)
	var mu sync.Mutex
	record := func(item jobresult.Item, itemErr error) {
		mu.Lock()
		defer mu.Unlock()
		switch item.Outcome {
		case jobresult.Succeeded:
			result.Succeeded++
		case jobresult.Skipped:
			result.Skipped++
			result.SkippedURLs = append(result.SkippedURLs, fault.FormatFailure(item.URL, "", errors.New(item.Err)))
		default:
			result.Failed++
			if item.Kind != "" {
				result.Kinds = append(result.Kinds, item.Kind)
			}
			if itemErr != nil {
				result.Errors = append(result.Errors, fault.FormatFailure(item.URL, item.Kind, itemErr))
			}
		}
		result.Items = append(result.Items, item)
	}

	var seen *workers.Dedupe
	if spec.Source.Follow != nil {
		seen = workers.NewDedupe()
	}
	for _, url := range spec.Source.URLs {
		if seen != nil && !seen.Claim(url) {
			continue
		}
		p.Submit(func() {
			e.runChain(ctx, spec, url, seen, record)
		})
	}
	p.Wait()

	return
}

// runChain scrapes start and, when source.follow is set, keeps following its
// next-page link one page at a time until there is none, it was already seen
// this job, or source.follow.maxPages pages of this chain have been fetched.
func (e *Executor) runChain(ctx context.Context, spec *jobspec.Spec, start string, seen *workers.Dedupe, record func(jobresult.Item, error)) {
	url, parentURL := start, ""
	for depth := 0; ; depth++ {
		links, _, kind, scrapeErr := e.scrape.Run(ctx, spec, url, parentURL, depth, nil)
		if scrapeErr != nil || kind != "" {
			errMsg := ""
			if scrapeErr != nil {
				errMsg = scrapeErr.Error()
			}
			record(jobresult.Item{URL: url, Outcome: jobresult.Failed, Kind: kind, Err: errMsg}, scrapeErr)
			return
		}
		record(jobresult.Item{URL: url, Outcome: jobresult.Succeeded}, nil)

		follow := spec.Source.Follow
		if follow == nil || len(links) == 0 || seen.Seen(links[0]) {
			return
		}
		if depth+1 >= follow.MaxPages {
			record(jobresult.Item{URL: links[0], Outcome: jobresult.Skipped, Err: "over --max-pages budget"}, nil)
			return
		}
		if !seen.Claim(links[0]) {
			return
		}
		url, parentURL = links[0], url
	}
}
