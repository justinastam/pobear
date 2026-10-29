package workers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/Decodo/pobear/pkg/fault"
	"github.com/Decodo/pobear/pkg/jobspec"
	"github.com/Decodo/pobear/services/core/internal/fetch/tiers"
	"github.com/Decodo/pobear/services/core/internal/sink/sinks"
)

type Fetcher interface {
	Fetch(ctx context.Context, cfg jobspec.Fetch, url string) (*tiers.Result, fault.Kind, error)
}

type Stager interface {
	Run(ctx context.Context, body []byte, cfgs []jobspec.Stage) (data map[string]any, err error)
}

type Sinker interface {
	Write(ctx context.Context, item sinks.Item) error
}

type LinkExtractor interface {
	Extract(body []byte, baseURL string) []string
}

type EmitGate func(finalURL string) (reason string, blocked bool)

type Scrape struct {
	fetcher Fetcher
	stager  Stager
	sinker  Sinker
	links   LinkExtractor
}

func NewScrape(f Fetcher, st Stager, sk Sinker, le LinkExtractor) *Scrape {
	return &Scrape{fetcher: f, stager: st, sinker: sk, links: le}
}

func (s *Scrape) Run(ctx context.Context, spec *jobspec.Spec, url, parentURL string, depth int, gate EmitGate) (links []string, blockedReason string, kind fault.Kind, err error) {
	result, fetchKind, fetchErr := s.fetcher.Fetch(ctx, spec.Fetch, url)
	if errors.Is(fetchErr, tiers.ErrRedirectBlocked) {
		return nil, tiers.ErrRedirectBlocked.Error(), "", nil
	}
	if fetchErr != nil || fetchKind != "" {
		return nil, "", fetchKind, fetchErr
	}

	if gate != nil && result.FinalURL != url {
		if reason, blocked := gate(result.FinalURL); blocked {
			return nil, reason, "", nil
		}
	}

	data, stageErr := s.stager.Run(ctx, result.Body, spec.Stages)
	if stageErr != nil {
		return nil, "", "", stageErr
	}

	item := sinks.Item{
		URL:       url,
		FinalURL:  result.FinalURL,
		ParentURL: parentURL,
		Status:    result.Status,
		Body:      string(result.Body),
		Depth:     depth,
		Data:      data,
		Tier:      result.Tier,
		Hash:      BodyHash(result.Body),
	}
	if err := s.sinker.Write(ctx, item); err != nil {
		return nil, "", "", err
	}

	return s.links.Extract(result.Body, result.FinalURL), "", "", nil
}

// BodyHash is the envelope's `hash` field: a SHA-256 of the body exactly as
// the envelope's `body` carries it, so two runs' items can be diffed by hash
// without comparing bodies.
func BodyHash(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}
