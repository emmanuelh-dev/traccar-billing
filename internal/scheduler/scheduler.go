package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
	"github.com/yourusername/traccar-billing/internal/traccar"
)

const tenantSyncTimeout = 30 * time.Second

type Scheduler struct {
	repo     billing.Repository
	client   billing.TraccarClient
	interval time.Duration
	logger   *slog.Logger
}

func New(repo billing.Repository, client billing.TraccarClient, interval time.Duration, logger *slog.Logger) *Scheduler {
	return &Scheduler{repo: repo, client: client, interval: interval, logger: logger}
}

func (s *Scheduler) Run(ctx context.Context) error {
	s.runOnce(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			s.runOnce(ctx)
		}
	}
}

func (s *Scheduler) runOnce(ctx context.Context) {
	tenants, err := s.repo.ListTenants(ctx)
	if err != nil {
		s.logger.Error("scheduler: list tenants", "error", err)
		return
	}

	for _, t := range tenants {
		tenantCtx, cancel := context.WithTimeout(ctx, tenantSyncTimeout)
		if err := s.SyncTenant(tenantCtx, t); err != nil {
			s.logger.Error("scheduler: sync tenant failed", "tenant_id", t.ID, "error", err)
		}
		if _, err := s.GenerateRemissions(tenantCtx, t, time.Now()); err != nil {
			s.logger.Error("scheduler: generate remissions failed", "tenant_id", t.ID, "error", err)
		}
		if err := s.checkOverdue(tenantCtx, t); err != nil {
			s.logger.Error("scheduler: check overdue failed", "tenant_id", t.ID, "error", err)
		}
		cancel()
	}
}

// GenerateRemissions issues one pending remission per calendar-billed
// subscription for the period containing now. It is safe to call repeatedly:
// the unique index on (subscription_id, period_start) makes a second run for
// the same month a no-op.
func (s *Scheduler) GenerateRemissions(ctx context.Context, t billing.Tenant, now time.Time) (int, error) {
	accounts, err := s.repo.ListAccountsByTenant(ctx, t.ID)
	if err != nil {
		return 0, fmt.Errorf("list accounts to generate remissions: %w", err)
	}

	createdCount := 0
	for _, acc := range accounts {
		if acc.Archived() {
			continue
		}
		// Skip temporary device-share users to avoid double-charging shared devices.
		if acc.Mirror() {
			continue
		}
		sub, err := s.repo.GetSubscriptionByAccountID(ctx, acc.ID)
		if errors.Is(err, billing.ErrNotFound) {
			continue
		}
		if err != nil {
			s.logger.Error("scheduler: get subscription for remission", "tenant_id", t.ID, "account_id", acc.ID, "error", err)
			continue
		}
		if !billing.ShouldBill(sub) {
			continue
		}

		rem := billing.BuildRemission(sub, acc, now)
		_, err = s.repo.CreateRemission(ctx, rem)
		if err != nil {
			if errors.Is(err, billing.ErrConflict) {
				continue
			}
			s.logger.Error("scheduler: create remission failed", "tenant_id", t.ID, "account_id", acc.ID, "error", err)
			continue
		}
		createdCount++
	}
	return createdCount, nil
}

func (s *Scheduler) SyncTenant(ctx context.Context, t billing.Tenant) error {
	if !t.HasValidSession(time.Now()) {
		s.logger.Warn("scheduler: tenant has no valid session, skipping sync", "tenant_id", t.ID)
		return nil
	}

	baseURL, err := url.Parse(t.BaseURL)
	if err != nil {
		return fmt.Errorf("parse base url: %w", err)
	}
	session := t.TraccarSession()

	users, err := s.client.FetchUsers(ctx, baseURL, session)
	if err != nil {
		return s.handleFetchError(ctx, t, "fetch users", err)
	}

	totalDevices := 0
	seen := make(map[int64]bool, len(users))
	for _, u := range users {
		devices, err := s.client.FetchDevicesForUser(ctx, baseURL, session, u.ID)
		if err != nil {
			return s.handleFetchError(ctx, t, "fetch devices", err)
		}
		totalDevices += len(devices)
		seen[u.ID] = true

		if _, err := s.repo.UpsertAccount(ctx, billing.Account{
			TenantID:      t.ID,
			TraccarUserID: u.ID,
			Name:          u.Name,
			Email:         u.Email,
			DeviceCount:   len(devices),
		}); err != nil {
			return fmt.Errorf("upsert account %d: %w", u.ID, err)
		}
	}

	archived, err := s.archiveMissing(ctx, t, seen)
	if err != nil {
		return err
	}

	s.logger.Info("scheduler: synced tenant", "tenant_id", t.ID, "users", len(users), "devices", totalDevices, "archived", archived)
	return nil
}

// archiveMissing runs only after a complete, successful user fetch: a
// partial list would otherwise archive every account it failed to see.
func (s *Scheduler) archiveMissing(ctx context.Context, t billing.Tenant, seen map[int64]bool) (int, error) {
	accounts, err := s.repo.ListAccountsByTenant(ctx, t.ID)
	if err != nil {
		return 0, fmt.Errorf("list accounts to archive: %w", err)
	}

	now := time.Now().UTC()
	archived := 0
	for _, account := range accounts {
		if seen[account.TraccarUserID] {
			continue
		}
		if err := s.repo.ArchiveAccount(ctx, account.ID, now); err != nil {
			return archived, fmt.Errorf("archive account %d: %w", account.ID, err)
		}
		s.logger.Info("scheduler: archived account, user gone from traccar", "tenant_id", t.ID, "account_id", account.ID, "traccar_user_id", account.TraccarUserID)
		archived++
	}
	return archived, nil
}

// handleFetchError clears the tenant's stored session on an expired/invalid
// Traccar cookie so the next /dashboard visit forces re-login, and swallows
// that specific case (expected steady state, not a run failure).
func (s *Scheduler) handleFetchError(ctx context.Context, t billing.Tenant, op string, err error) error {
	if errors.Is(err, traccar.ErrUnauthorized) {
		s.logger.Warn("scheduler: tenant session expired, clearing", "tenant_id", t.ID)
		return s.repo.UpdateTenantSession(ctx, t.ID, billing.Session{})
	}
	return fmt.Errorf("%s: %w", op, err)
}

func (s *Scheduler) checkOverdue(ctx context.Context, t billing.Tenant) error {
	now := time.Now()
	subs, err := s.repo.ListSubscriptionsDueBefore(ctx, t.ID, now)
	if err != nil {
		return fmt.Errorf("list due subscriptions: %w", err)
	}

	for _, sub := range subs {
		if !billing.IsOverdue(sub, now) {
			continue
		}
		if sub.Status != billing.StatusOverdue {
			sub.Status = billing.StatusOverdue
			if _, err := s.repo.UpsertSubscription(ctx, sub); err != nil {
				return fmt.Errorf("mark subscription %d overdue: %w", sub.ID, err)
			}
		}
		// Retried every tick, not just on the transition, so a Traccar
		// outage during the first attempt self-heals on the next pass.
		s.pauseTraccarUser(ctx, t, sub.AccountID)
	}
	return nil
}

func (s *Scheduler) pauseTraccarUser(ctx context.Context, t billing.Tenant, accountID int64) {
	if !t.HasValidSession(time.Now()) {
		s.logger.Warn("scheduler: cannot pause traccar user, tenant has no valid session", "tenant_id", t.ID, "account_id", accountID)
		return
	}

	account, err := s.repo.GetAccount(ctx, t.ID, accountID)
	if err != nil {
		s.logger.Error("scheduler: get account to pause traccar user", "account_id", accountID, "error", err)
		return
	}
	if account.TraccarUserID == t.AdminTraccarUserID {
		s.logger.Warn("scheduler: refusing to pause the tenant's own admin user", "tenant_id", t.ID, "account_id", accountID)
		return
	}

	baseURL, err := url.Parse(t.BaseURL)
	if err != nil {
		s.logger.Error("scheduler: parse base url to pause user", "error", err)
		return
	}
	session := t.TraccarSession()

	if err := s.client.SetUserDisabled(ctx, baseURL, session, account.TraccarUserID, true); err != nil {
		s.logger.Error("scheduler: pause traccar user", "account_id", accountID, "traccar_user_id", account.TraccarUserID, "error", err)
		return
	}
	s.logger.Info("scheduler: paused traccar user for overdue account", "account_id", accountID, "traccar_user_id", account.TraccarUserID)
}
