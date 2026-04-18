package sync

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"

	"github.com/dvflw/mantle/internal/repo"
)

// Poller owns the background goroutines that drive auto-apply syncs.
// Construct one per process (typically from `mantle serve`) and call
// Run with a cancellable context. Run returns when ctx is done.
type Poller struct {
	DB     *sql.DB
	Store  *repo.Store
	Driver Driver

	// OnSync, if set, is called after every sync attempt. Useful for
	// metrics emission and tests; production wiring leaves it nil.
	OnSync func(*repo.Repo, *Report, error)

	// MinInterval lets tests override the 10s minimum poll_interval
	// floor. Production leaves it zero; the poller uses 10s as the
	// safety floor.
	MinInterval time.Duration
}

// Run starts one goroutine per enabled, auto-apply repo. Each ticks at
// the repo's poll_interval and calls SyncRepo. Returns when ctx is
// cancelled — all child goroutines exit within one tick.
func (p *Poller) Run(ctx context.Context) {
	repos, err := p.Store.List(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "poller: list repos failed", "err", err)
		return
	}

	var wg sync.WaitGroup
	for i := range repos {
		r := repos[i]
		if !r.Enabled || !r.AutoApply {
			continue
		}
		wg.Add(1)
		go func(r repo.Repo) {
			defer wg.Done()
			p.tickLoop(ctx, &r)
		}(r)
	}
	wg.Wait()
}

func (p *Poller) tickLoop(ctx context.Context, r *repo.Repo) {
	interval, err := time.ParseDuration(r.PollInterval)
	if err != nil {
		slog.ErrorContext(ctx, "poller: bad poll_interval", "repo", r.Name, "value", r.PollInterval, "err", err)
		return
	}
	// MinInterval overrides the safety floor. In production it is zero and the
	// floor is 10s. In tests it is set to a small value so ticks fire quickly
	// without requiring a stored interval below the store's validation minimum.
	floor := p.MinInterval
	if floor == 0 {
		floor = 10 * time.Second
	}
	if interval < floor {
		interval = floor
	}
	// When MinInterval is explicitly set (test mode) use it as the actual tick
	// rate, bypassing the stored interval entirely, so timing-sensitive tests
	// don't have to store sub-10s intervals that would fail store validation.
	if p.MinInterval != 0 {
		interval = p.MinInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			report, syncErr := SyncRepo(ctx, p.DB, p.Store, r, p.Driver)
			if p.OnSync != nil {
				p.OnSync(r, report, syncErr)
			}
		}
	}
}
