package sources

import (
	"bufio"
	"context"
	"errors"
	"net/http"

	"github.com/subfinder/research/core"
	"golang.org/x/sync/semaphore"
)

// Threatminer is a source to process subdomains from https://www.threatminer.org
type Threatminer struct {
	lock *semaphore.Weighted
}

// ProcessDomain takes a given base domain and attempts to enumerate subdomains.
func (source *Threatminer) ProcessDomain(ctx context.Context, domain string) <-chan *core.Result {
	if source.lock == nil {
		source.lock = defaultLockValue()
	}

	results := make(chan *core.Result)

	go func(domain string, results chan *core.Result) {
		defer close(results)

		if err := source.lock.Acquire(ctx, 1); err != nil {
			sendResultWithContext(ctx, results, core.NewResult(threatminerLabel, nil, err))
			return
		}
		defer source.lock.Release(1)

		domainExtractor := core.NewSingleSubdomainExtractor(domain)

		req, err := http.NewRequest(http.MethodGet, "https://www.threatminer.org/getData.php?e=subdomains_container&q="+domain+"&t=0&rt=10&p=1", nil)
		if err != nil {
			sendResultWithContext(ctx, results, core.NewResult(threatminerLabel, nil, err))
			return
		}

		req.Cancel = ctx.Done()
		req.WithContext(ctx)

		resp, err := core.HTTPClient.Do(req)
		if err != nil {
			sendResultWithContext(ctx, results, core.NewResult(threatminerLabel, nil, err))
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			sendResultWithContext(ctx, results, core.NewResult(threatminerLabel, nil, errors.New(resp.Status)))
			return
		}

		scanner := bufio.NewScanner(resp.Body)

		scanner.Split(bufio.ScanWords)

		for scanner.Scan() {
			if ctx.Err() != nil {
				return
			}
			str := domainExtractor(scanner.Bytes())
			if str != "" {
				if !sendResultWithContext(ctx, results, core.NewResult(threatminerLabel, str, nil)) {
					return
				}
			}
		}

		err = scanner.Err()

		if err != nil {
			sendResultWithContext(ctx, results, core.NewResult(threatminerLabel, nil, err))
			return
		}

	}(domain, results)
	return results
}
