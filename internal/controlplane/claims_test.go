package controlplane

// C5 tests (WS-20261015-001): concurrent claim race, lease expiry with an
// injected clock, restart persistence, and heartbeat/release semantics.

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type claimTestEnv struct {
	repo  *Repository
	store *ClaimStore
	clock func() time.Time
	// advance moves the injected clock forward.
	advance func(d time.Duration)
	close   func()
}

func newClaimTestEnv(t *testing.T) *claimTestEnv {
	t.Helper()
	repo, err := Open(filepath.Join(t.TempDir(), "claims.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	var clockValue atomic.Value
	clockValue.Store(time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC))
	clock := func() time.Time { return clockValue.Load().(time.Time) }
	return &claimTestEnv{
		repo:    repo,
		store:   NewClaimStore(repo, clock),
		clock:   clock,
		advance: func(d time.Duration) { clockValue.Store(clock().Add(d)) },
		close:   func() { repo.Close() },
	}
}

func TestClaimReadyTaskConcurrentRace(t *testing.T) {
	env := newClaimTestEnv(t)
	defer env.close()
	ctx := context.Background()

	const goroutines = 10
	var successes int32
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	start := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			<-start
			claim, err := env.store.ClaimReadyTask(ctx, "ian", []string{"task-1"}, "runtime-"+string(rune('a'+n)), "hermes")
			if errors.Is(err, ErrNotFound) {
				return
			}
			if err != nil {
				errs <- err
				return
			}
			if claim.TaskID != "task-1" {
				errs <- errors.New("claimed wrong task: " + claim.TaskID)
				return
			}
			atomic.AddInt32(&successes, 1)
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("unexpected claim error: %v", err)
	}
	if got := atomic.LoadInt32(&successes); got != 1 {
		t.Fatalf("concurrent claims succeeded = %d, want exactly 1", got)
	}
	// After the winner holds the lease, a retry finds nothing claimable.
	if _, err := env.store.ClaimReadyTask(ctx, "ian", []string{"task-1"}, "runtime-z", "hermes"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second claim err = %v, want ErrNotFound", err)
	}
}

func TestClaimLeaseExpiryWithInjectedClock(t *testing.T) {
	env := newClaimTestEnv(t)
	defer env.close()
	ctx := context.Background()

	claim, err := env.store.ClaimReadyTask(ctx, "ian", []string{"task-1"}, "runtime-1", "hermes")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if want := env.clock().Add(LeaseTTL); !claim.LeaseExpiresAt.Equal(want) {
		t.Fatalf("leaseExpiresAt = %s, want %s (now+120s)", claim.LeaseExpiresAt, want)
	}

	// Advance past the TTL: heartbeat must refuse (expired leases re-claim).
	env.advance(LeaseTTL + time.Second)
	if _, err := env.store.HeartbeatClaim(ctx, "ian", claim.ClaimID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("heartbeat on expired lease err = %v, want ErrNotFound", err)
	}

	// Expiry sweep marks the lease expired.
	expired, err := env.store.ExpireStaleClaims(ctx)
	if err != nil {
		t.Fatalf("ExpireStaleClaims: %v", err)
	}
	if len(expired) != 1 || expired[0].Claim.ClaimID != claim.ClaimID {
		t.Fatalf("expired = %+v, want exactly lease %s", expired, claim.ClaimID)
	}
	if expired[0].Claim.Status != ClaimStatusExpired {
		t.Fatalf("status = %s, want expired", expired[0].Claim.Status)
	}
	if expired[0].Timeline.TaskID != "task-1" || expired[0].Timeline.Action != "task_lease_expired" {
		t.Fatalf("timeline event = %+v, want task_lease_expired for task-1", expired[0].Timeline)
	}

	// The freed task can be claimed again by a different runtime.
	reclaim, err := env.store.ClaimReadyTask(ctx, "ian", []string{"task-1"}, "runtime-2", "codex")
	if err != nil {
		t.Fatalf("re-claim after expiry: %v", err)
	}
	if reclaim.ClaimID == claim.ClaimID {
		t.Fatal("re-claim reused the expired lease id")
	}
}

func TestHeartbeatRenewsLease(t *testing.T) {
	env := newClaimTestEnv(t)
	defer env.close()
	ctx := context.Background()

	claim, err := env.store.ClaimReadyTask(ctx, "ian", []string{"task-1"}, "runtime-1", "hermes")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	env.advance(60 * time.Second)
	result, err := env.store.HeartbeatClaim(ctx, "ian", claim.ClaimID)
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if want := env.clock().Add(LeaseTTL); !result.LeaseExpiresAt.Equal(want) {
		t.Fatalf("renewed leaseExpiresAt = %s, want %s", result.LeaseExpiresAt, want)
	}
	if result.Timeline.Action != "task_lease_heartbeat" {
		t.Fatalf("timeline action = %s, want task_lease_heartbeat", result.Timeline.Action)
	}

	// Unknown lease ids and foreign usernames are 404s, never renewals.
	if _, err := env.store.HeartbeatClaim(ctx, "ian", "claim-does-not-exist"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown lease err = %v, want ErrNotFound", err)
	}
	if _, err := env.store.HeartbeatClaim(ctx, "mallory", claim.ClaimID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign username err = %v, want ErrNotFound", err)
	}
}

func TestReleaseClaimOutcomeMapping(t *testing.T) {
	env := newClaimTestEnv(t)
	defer env.close()
	ctx := context.Background()

	// Validation first: bad outcome is rejected before touching storage.
	if _, err := env.store.ReleaseClaim(ctx, "ian", "claim-x", "abandoned"); err == nil {
		t.Fatal("invalid outcome accepted")
	}

	claim, err := env.store.ClaimReadyTask(ctx, "ian", []string{"task-1"}, "runtime-1", "hermes")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	env.advance(30 * time.Second)

	result, err := env.store.ReleaseClaim(ctx, "ian", claim.ClaimID, ReleaseOutcomeDone)
	if err != nil {
		t.Fatalf("release done: %v", err)
	}
	if result.NextDispatchState != DispatchStateDone {
		t.Fatalf("done outcome -> dispatchState %s, want done", result.NextDispatchState)
	}
	if result.Claim.Status != ClaimStatusReleased {
		t.Fatalf("status = %s, want released", result.Claim.Status)
	}

	// failed / released outcomes send the task back to ready (re-claimable).
	second, err := env.store.ClaimReadyTask(ctx, "ian", []string{"task-2"}, "runtime-1", "hermes")
	if err != nil {
		t.Fatalf("claim task-2: %v", err)
	}
	result, err = env.store.ReleaseClaim(ctx, "ian", second.ClaimID, ReleaseOutcomeFailed)
	if err != nil {
		t.Fatalf("release failed: %v", err)
	}
	if result.NextDispatchState != DispatchStateReady {
		t.Fatalf("failed outcome -> dispatchState %s, want ready", result.NextDispatchState)
	}

	// A released lease cannot be released or heartbeated again.
	if _, err := env.store.ReleaseClaim(ctx, "ian", claim.ClaimID, ReleaseOutcomeDone); !errors.Is(err, ErrNotFound) {
		t.Fatalf("double release err = %v, want ErrNotFound", err)
	}
	if _, err := env.store.HeartbeatClaim(ctx, "ian", claim.ClaimID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("heartbeat after release err = %v, want ErrNotFound", err)
	}
}

func TestClaimLeasesSurviveRestart(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "restart.sqlite")
	var clockValue atomic.Value
	start := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	clockValue.Store(start)
	clock := func() time.Time { return clockValue.Load().(time.Time) }

	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("open 1: %v", err)
	}
	store := NewClaimStore(repo, clock)
	claim, err := store.ClaimReadyTask(context.Background(), "ian", []string{"task-1"}, "runtime-1", "hermes")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("close 1: %v", err)
	}

	// "Restart": reopen the database and verify the lease is still active,
	// still inside its TTL, and the task is not re-claimable.
	repo2, err := Open(dir)
	if err != nil {
		t.Fatalf("open 2: %v", err)
	}
	defer repo2.Close()
	store2 := NewClaimStore(repo2, clock)

	got, err := store2.GetClaim(context.Background(), "ian", claim.ClaimID)
	if err != nil {
		t.Fatalf("get after restart: %v", err)
	}
	if got.Status != ClaimStatusActive {
		t.Fatalf("status after restart = %s, want active", got.Status)
	}
	if !got.LeaseExpiresAt.Equal(claim.LeaseExpiresAt) {
		t.Fatalf("leaseExpiresAt after restart = %s, want %s", got.LeaseExpiresAt, claim.LeaseExpiresAt)
	}
	active, err := store2.ActiveClaimForTask(context.Background(), "ian", "task-1")
	if err != nil {
		t.Fatalf("active claim after restart: %v", err)
	}
	if active.ClaimID != claim.ClaimID {
		t.Fatalf("active claim id = %s, want %s", active.ClaimID, claim.ClaimID)
	}
	if _, err := store2.ClaimReadyTask(context.Background(), "ian", []string{"task-1"}, "runtime-2", "codex"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("claim after restart err = %v, want ErrNotFound", err)
	}

	// And an expired lease is recycled by the sweep after the restart too.
	clockValue.Store(start.Add(LeaseTTL + time.Minute))
	expired, err := store2.ExpireStaleClaims(context.Background())
	if err != nil {
		t.Fatalf("expire after restart: %v", err)
	}
	if len(expired) != 1 {
		t.Fatalf("expired after restart = %d items, want 1", len(expired))
	}
}

func TestExpireStaleClaimsNoopWhenFresh(t *testing.T) {
	env := newClaimTestEnv(t)
	defer env.close()
	ctx := context.Background()
	if _, err := env.store.ClaimReadyTask(ctx, "ian", []string{"task-1"}, "runtime-1", "hermes"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	expired, err := env.store.ExpireStaleClaims(ctx)
	if err != nil {
		t.Fatalf("ExpireStaleClaims: %v", err)
	}
	if len(expired) != 0 {
		t.Fatalf("fresh lease expired = %d items, want 0", len(expired))
	}
}
