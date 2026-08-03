package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
)

// sqlRepository implements billing.Repository against any database/sql
// driver using "?" placeholders (both go-sql-driver/mysql and
// modernc.org/sqlite share that style, so the query text is identical
// across dialects except for the two upsert statements below).
type sqlRepository struct {
	db      *sql.DB
	dialect string // "mysql" or "sqlite"
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

func scanTime(nt sql.NullTime) time.Time {
	if !nt.Valid {
		return time.Time{}
	}
	return nt.Time
}

func (r *sqlRepository) CreateTenant(ctx context.Context, t billing.Tenant) (billing.Tenant, error) {
	now := time.Now().UTC()
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO tenants (name, base_url, session_cookie, session_expires_at, admin_traccar_user_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.Name, t.BaseURL, t.SessionCookie, nullTime(t.SessionExpiresAt), t.AdminTraccarUserID, now, now)
	if err != nil {
		return billing.Tenant{}, fmt.Errorf("storage: create tenant: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return billing.Tenant{}, fmt.Errorf("storage: create tenant: %w", err)
	}
	return r.GetTenantByID(ctx, id)
}

func (r *sqlRepository) GetTenantByID(ctx context.Context, id int64) (billing.Tenant, error) {
	return r.scanTenant(r.db.QueryRowContext(ctx,
		`SELECT id, name, base_url, session_cookie, session_expires_at, admin_traccar_user_id, created_at, updated_at
		 FROM tenants WHERE id = ?`, id))
}

func (r *sqlRepository) GetTenantByBaseURL(ctx context.Context, baseURL string) (billing.Tenant, error) {
	return r.scanTenant(r.db.QueryRowContext(ctx,
		`SELECT id, name, base_url, session_cookie, session_expires_at, admin_traccar_user_id, created_at, updated_at
		 FROM tenants WHERE base_url = ?`, baseURL))
}

func (r *sqlRepository) scanTenant(row *sql.Row) (billing.Tenant, error) {
	var t billing.Tenant
	var sessionExpiresAt sql.NullTime
	err := row.Scan(&t.ID, &t.Name, &t.BaseURL, &t.SessionCookie, &sessionExpiresAt, &t.AdminTraccarUserID, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return billing.Tenant{}, billing.ErrNotFound
	}
	if err != nil {
		return billing.Tenant{}, fmt.Errorf("storage: scan tenant: %w", err)
	}
	t.SessionExpiresAt = scanTime(sessionExpiresAt)
	return t, nil
}

func (r *sqlRepository) UpdateTenantSession(ctx context.Context, tenantID int64, session billing.Session) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE tenants SET session_cookie = ?, session_expires_at = ?, updated_at = ? WHERE id = ?`,
		session.Cookie, nullTime(session.ExpiresAt), time.Now().UTC(), tenantID)
	if err != nil {
		return fmt.Errorf("storage: update tenant session: %w", err)
	}
	return nil
}

func (r *sqlRepository) UpdateTenantAdmin(ctx context.Context, tenantID int64, adminTraccarUserID int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE tenants SET admin_traccar_user_id = ?, updated_at = ? WHERE id = ?`,
		adminTraccarUserID, time.Now().UTC(), tenantID)
	if err != nil {
		return fmt.Errorf("storage: update tenant admin: %w", err)
	}
	return nil
}

func (r *sqlRepository) ListTenants(ctx context.Context) ([]billing.Tenant, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, base_url, session_cookie, session_expires_at, admin_traccar_user_id, created_at, updated_at
		 FROM tenants ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("storage: list tenants: %w", err)
	}
	defer rows.Close()

	var tenants []billing.Tenant
	for rows.Next() {
		var t billing.Tenant
		var sessionExpiresAt sql.NullTime
		if err := rows.Scan(&t.ID, &t.Name, &t.BaseURL, &t.SessionCookie, &sessionExpiresAt, &t.AdminTraccarUserID, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("storage: scan tenant: %w", err)
		}
		t.SessionExpiresAt = scanTime(sessionExpiresAt)
		tenants = append(tenants, t)
	}
	return tenants, rows.Err()
}

func (r *sqlRepository) UpsertAccount(ctx context.Context, a billing.Account) (billing.Account, error) {
	now := time.Now().UTC()

	var query string
	if r.dialect == "mysql" {
		query = `INSERT INTO accounts (tenant_id, traccar_user_id, name, email, device_count, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE name = VALUES(name), email = VALUES(email), device_count = VALUES(device_count), updated_at = VALUES(updated_at)`
	} else {
		query = `INSERT INTO accounts (tenant_id, traccar_user_id, name, email, device_count, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(tenant_id, traccar_user_id) DO UPDATE SET name = excluded.name, email = excluded.email, device_count = excluded.device_count, updated_at = excluded.updated_at`
	}

	if _, err := r.db.ExecContext(ctx, query, a.TenantID, a.TraccarUserID, a.Name, a.Email, a.DeviceCount, now, now); err != nil {
		return billing.Account{}, fmt.Errorf("storage: upsert account: %w", err)
	}

	return r.scanAccount(r.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, traccar_user_id, name, email, device_count, created_at, updated_at
		 FROM accounts WHERE tenant_id = ? AND traccar_user_id = ?`, a.TenantID, a.TraccarUserID))
}

func (r *sqlRepository) GetAccount(ctx context.Context, tenantID, accountID int64) (billing.Account, error) {
	return r.scanAccount(r.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, traccar_user_id, name, email, device_count, created_at, updated_at
		 FROM accounts WHERE tenant_id = ? AND id = ?`, tenantID, accountID))
}

func (r *sqlRepository) scanAccount(row *sql.Row) (billing.Account, error) {
	var a billing.Account
	err := row.Scan(&a.ID, &a.TenantID, &a.TraccarUserID, &a.Name, &a.Email, &a.DeviceCount, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return billing.Account{}, billing.ErrNotFound
	}
	if err != nil {
		return billing.Account{}, fmt.Errorf("storage: scan account: %w", err)
	}
	return a, nil
}

func (r *sqlRepository) ListAccountsByTenant(ctx context.Context, tenantID int64) ([]billing.Account, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, tenant_id, traccar_user_id, name, email, device_count, created_at, updated_at
		 FROM accounts WHERE tenant_id = ? ORDER BY id`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("storage: list accounts: %w", err)
	}
	defer rows.Close()

	var accounts []billing.Account
	for rows.Next() {
		var a billing.Account
		if err := rows.Scan(&a.ID, &a.TenantID, &a.TraccarUserID, &a.Name, &a.Email, &a.DeviceCount, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("storage: scan account: %w", err)
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

func (r *sqlRepository) UpsertSubscription(ctx context.Context, s billing.Subscription) (billing.Subscription, error) {
	now := time.Now().UTC()

	if s.ID == 0 {
		res, err := r.db.ExecContext(ctx,
			`INSERT INTO subscriptions (account_id, status, amount_cents, currency, period_days, last_paid_at, next_due_at, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			s.AccountID, string(s.Status), s.AmountCents, s.Currency, s.PeriodDays, nullTime(s.LastPaidAt), s.NextDueAt, now, now)
		if err != nil {
			return billing.Subscription{}, fmt.Errorf("storage: create subscription: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return billing.Subscription{}, fmt.Errorf("storage: create subscription: %w", err)
		}
		return r.GetSubscription(ctx, id)
	}

	if _, err := r.db.ExecContext(ctx,
		`UPDATE subscriptions SET status = ?, amount_cents = ?, currency = ?, period_days = ?, last_paid_at = ?, next_due_at = ?, updated_at = ?
		 WHERE id = ?`,
		string(s.Status), s.AmountCents, s.Currency, s.PeriodDays, nullTime(s.LastPaidAt), s.NextDueAt, now, s.ID); err != nil {
		return billing.Subscription{}, fmt.Errorf("storage: update subscription: %w", err)
	}
	return r.GetSubscription(ctx, s.ID)
}

func (r *sqlRepository) GetSubscription(ctx context.Context, id int64) (billing.Subscription, error) {
	return r.scanSubscription(r.db.QueryRowContext(ctx,
		`SELECT id, account_id, status, amount_cents, currency, period_days, last_paid_at, next_due_at, created_at, updated_at
		 FROM subscriptions WHERE id = ?`, id))
}

func (r *sqlRepository) GetSubscriptionByAccountID(ctx context.Context, accountID int64) (billing.Subscription, error) {
	return r.scanSubscription(r.db.QueryRowContext(ctx,
		`SELECT id, account_id, status, amount_cents, currency, period_days, last_paid_at, next_due_at, created_at, updated_at
		 FROM subscriptions WHERE account_id = ?`, accountID))
}

func (r *sqlRepository) scanSubscription(row *sql.Row) (billing.Subscription, error) {
	var s billing.Subscription
	var status string
	var lastPaidAt sql.NullTime
	err := row.Scan(&s.ID, &s.AccountID, &status, &s.AmountCents, &s.Currency, &s.PeriodDays, &lastPaidAt, &s.NextDueAt, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return billing.Subscription{}, billing.ErrNotFound
	}
	if err != nil {
		return billing.Subscription{}, fmt.Errorf("storage: scan subscription: %w", err)
	}
	s.Status = billing.SubscriptionStatus(status)
	s.LastPaidAt = scanTime(lastPaidAt)
	return s, nil
}

func (r *sqlRepository) ListSubscriptionsDueBefore(ctx context.Context, tenantID int64, cutoff time.Time) ([]billing.Subscription, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT s.id, s.account_id, s.status, s.amount_cents, s.currency, s.period_days, s.last_paid_at, s.next_due_at, s.created_at, s.updated_at
		 FROM subscriptions s
		 JOIN accounts a ON a.id = s.account_id
		 WHERE a.tenant_id = ? AND s.next_due_at < ?
		 ORDER BY s.next_due_at`, tenantID, cutoff)
	if err != nil {
		return nil, fmt.Errorf("storage: list due subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []billing.Subscription
	for rows.Next() {
		var s billing.Subscription
		var status string
		var lastPaidAt sql.NullTime
		if err := rows.Scan(&s.ID, &s.AccountID, &status, &s.AmountCents, &s.Currency, &s.PeriodDays, &lastPaidAt, &s.NextDueAt, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("storage: scan subscription: %w", err)
		}
		s.Status = billing.SubscriptionStatus(status)
		s.LastPaidAt = scanTime(lastPaidAt)
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

func (r *sqlRepository) RecordPayment(ctx context.Context, p billing.Payment) (billing.Payment, error) {
	now := time.Now().UTC()
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO payments (subscription_id, amount_cents, currency, paid_at, note, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		p.SubscriptionID, p.AmountCents, p.Currency, p.PaidAt, p.Note, now)
	if err != nil {
		return billing.Payment{}, fmt.Errorf("storage: record payment: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return billing.Payment{}, fmt.Errorf("storage: record payment: %w", err)
	}

	var pmt billing.Payment
	row := r.db.QueryRowContext(ctx,
		`SELECT id, subscription_id, amount_cents, currency, paid_at, note, created_at FROM payments WHERE id = ?`, id)
	if err := row.Scan(&pmt.ID, &pmt.SubscriptionID, &pmt.AmountCents, &pmt.Currency, &pmt.PaidAt, &pmt.Note, &pmt.CreatedAt); err != nil {
		return billing.Payment{}, fmt.Errorf("storage: scan payment: %w", err)
	}
	return pmt, nil
}

func (r *sqlRepository) ListPaymentsBySubscription(ctx context.Context, subscriptionID int64) ([]billing.Payment, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, subscription_id, amount_cents, currency, paid_at, note, created_at
		 FROM payments WHERE subscription_id = ? ORDER BY paid_at DESC`, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("storage: list payments: %w", err)
	}
	defer rows.Close()

	var payments []billing.Payment
	for rows.Next() {
		var p billing.Payment
		if err := rows.Scan(&p.ID, &p.SubscriptionID, &p.AmountCents, &p.Currency, &p.PaidAt, &p.Note, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("storage: scan payment: %w", err)
		}
		payments = append(payments, p)
	}
	return payments, rows.Err()
}
