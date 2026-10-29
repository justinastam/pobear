package command

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	"github.com/Decodo/pobear/pkg/exitcode"
	"github.com/Decodo/pobear/pkg/fault"
	"github.com/Decodo/pobear/pkg/jobresult"
	"github.com/Decodo/pobear/pkg/jobspec"
	"github.com/Decodo/pobear/services/cli/internal/jobgen"
	"github.com/Decodo/pobear/services/cli/internal/providerator"
	"github.com/Decodo/pobear/services/core"
)

type RunResult = jobresult.Result

type JobRunner interface {
	Run(ctx context.Context, data []byte, mode jobspec.Mode) (RunResult, fault.Kind, error)
}

type coreRunner struct{ svc *core.Service }

func NewJobRunner(svc *core.Service) JobRunner {
	return coreRunner{svc: svc}
}

func (r coreRunner) Run(ctx context.Context, data []byte, mode jobspec.Mode) (RunResult, fault.Kind, error) {
	return r.svc.Run(ctx, data, mode)
}

type Scrape struct {
	Runner          JobRunner
	Providers       *providerator.Providerator
	CredentialsPath string
}

func (s *Scrape) Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var f commonFlags
	var follow string
	var maxPages int
	code := 0

	cmd := &cobra.Command{
		Use:   "scrape <url|file>",
		Short: (&Scrape{}).Summary(),
		Long: "Scrape a single URL, or every URL listed in a file.\n\n" +
			"The positional argument is either a single absolute http/https URL, or a\n" +
			"path to a file listing many: plain text, one URL per line, blank lines and\n" +
			"lines starting with # skipped.\n\n" +
			"--follow turns each URL into the start of a pagination chain: every page's\n" +
			"next-page link (the first match of the selector) is fetched in turn, until\n" +
			"there's no next link, the link was already fetched, or --max-pages is hit.",
		Args: requireExactlyOne("scrape requires a URL or file argument"),
		RunE: func(cobraCmd *cobra.Command, cmdArgs []string) error {
			followIn, ferr := resolveFollow(follow, maxPages, cobraCmd.Flags().Changed("max-pages"))
			if ferr != nil {
				return ferr
			}
			c, cerr := scrapeRun(ctx, s.Runner, s.Providers, s.CredentialsPath, cmdArgs[0], &f, followIn, stdout, stderr)
			code = c
			if cerr != nil {
				return cerr
			}
			return nil
		},
	}
	registerCommonFlags(cmd, &f, 0)
	flags := cmd.Flags()
	flags.StringVar(&follow, "follow", "", "follow a next-page link chain from each URL, as selector[@attr] (attr defaults to href)")
	flags.IntVar(&maxPages, "max-pages", 100, "with --follow: stop each chain after fetching this many pages")
	wireProviderHelp(cmd, s.Providers, &f.provider)

	return runCobraCommand(cmd, args, stdin, stdout, stderr, &code)
}

func resolveFollow(selector string, maxPages int, maxPagesSet bool) (*jobgen.FollowInput, *cliError) {
	if selector == "" {
		if maxPagesSet {
			return nil, newCLIError(exitcode.UsageError, "--max-pages requires --follow")
		}
		return nil, nil
	}
	return &jobgen.FollowInput{Selector: selector, MaxPages: &maxPages}, nil
}

func scrapeRun(ctx context.Context, runner JobRunner, providers *providerator.Providerator, credentialsPath, target string, f *commonFlags, followIn *jobgen.FollowInput, stdout, stderr io.Writer) (int, *cliError) {
	urls, cerr := resolveScrapeURLs(target)
	if cerr != nil {
		return 0, cerr
	}

	stages, cerr := buildStages(f.extract, f.css)
	if cerr != nil {
		return 0, cerr
	}

	names := extractedFieldNames(f.extract, f.css)
	if cerr := validateBare(f.bare, names); cerr != nil {
		return 0, cerr
	}
	if cerr := validateBareSingleTarget(f.bare, isDirOut(f.out), len(urls)); cerr != nil {
		return 0, cerr
	}
	if cerr := validateBareFollow(f.bare, isDirOut(f.out), followIn); cerr != nil {
		return 0, cerr
	}

	fetchUse, cerr := resolveFetchUse(f)
	if cerr != nil {
		return 0, cerr
	}

	opts := mergeOptions(f.render, f.geo, f.opts)
	var fetchOptions map[string]any
	if f.provider != "" {
		var perr *cliError
		fetchOptions, perr = resolveProviderOptions(providers, credentialsPath, f.provider, f.template, opts, !f.printSpec && !f.printSpecVerbose)
		if perr != nil {
			return 0, perr
		}
	} else {
		fetchOptions = stringMapToAny(opts)
	}

	sinkUse, sinkPath := "stdout", ""
	if f.out != "" && !isDirOut(f.out) {
		sinkUse, sinkPath = "ndjson", f.out
	}

	profile, cerr := resolveProfile(f, fetchUse)
	if cerr != nil {
		return 0, cerr
	}

	headers, cerr := buildHeaders(f.headers)
	if cerr != nil {
		return 0, cerr
	}

	resolvedIn := jobgen.Input{
		URLs: urls, Follow: followIn, FetchUse: fetchUse, Ladder: f.ladder, Proxy: f.proxy, Provider: f.provider, Template: f.template,
		Profile: profile, Options: fetchOptions, Timeout: f.timeout, Headers: headers, Stages: stages,
		Concurrency: &f.concurrency, RPS: &f.rps, Retries: &f.retries, SinkUse: sinkUse, SinkPath: sinkPath,
	}
	defaultsIn := jobgen.Input{URLs: urls, FetchUse: "local"}
	if followIn != nil {
		defaultsIn.Follow = &jobgen.FollowInput{Selector: followIn.Selector}
	}

	rawField := ""
	if len(names) == 1 {
		rawField = names[0]
	}

	return printSpecOrRun(ctx, runner, resolvedIn, defaultsIn, f.printSpec, f.printSpecVerbose, f.bare, f.failOnEmpty, rawField, f.out, stdout, stderr)
}

func (*Scrape) Summary() string { return "Scrape a single URL, or every URL listed in a file" }
