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
	tx      *sql.Tx
	dialect string // "mysql" or "sqlite"
}

type dbtx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (r *sqlRepository) q() dbtx {
	if r.tx != nil {
		return r.tx
	}
	return r.db
}

func (r *sqlRepository) WithTx(ctx context.Context, fn func(billing.Repository) error) error {
	if r.tx != nil {
		return fn(r)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("storage: begin transaction: %w", err)
	}

	if err := fn(&sqlRepository{db: r.db, tx: tx, dialect: r.dialect}); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			return fmt.Errorf("storage: rollback after %v: %w", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("storage: commit transaction: %w", err)
	}
	return nil
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
	res, err := r.q().ExecContext(ctx,
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
	return r.scanTenant(r.q().QueryRowContext(ctx,
		`SELECT id, name, base_url, session_cookie, session_expires_at, admin_traccar_user_id, created_at, updated_at
		 FROM tenants WHERE id = ?`, id))
}

func (r *sqlRepository) GetTenantByBaseURL(ctx context.Context, baseURL string) (billing.Tenant, error) {
	return r.scanTenant(r.q().QueryRowContext(ctx,
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
	_, err := r.q().ExecContext(ctx,
		`UPDATE tenants SET session_cookie = ?, session_expires_at = ?, updated_at = ? WHERE id = ?`,
		session.Cookie, nullTime(session.ExpiresAt), time.Now().UTC(), tenantID)
	if err != nil {
		return fmt.Errorf("storage: update tenant session: %w", err)
	}
	return nil
}

func (r *sqlRepository) UpdateTenantAdmin(ctx context.Context, tenantID int64, adminTraccarUserID int64) error {
	_, err := r.q().ExecContext(ctx,
		`UPDATE tenants SET admin_traccar_user_id = ?, updated_at = ? WHERE id = ?`,
		adminTraccarUserID, time.Now().UTC(), tenantID)
	if err != nil {
		return fmt.Errorf("storage: update tenant admin: %w", err)
	}
	return nil
}

func (r *sqlRepository) ListTenants(ctx context.Context) ([]billing.Tenant, error) {
	rows, err := r.q().QueryContext(ctx,
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

const accountColumns = `id, tenant_id, traccar_user_id, name, email, device_count, seller_id, archived_at, created_at, updated_at`

func scanAccountInto(sc scanner) (billing.Account, error) {
	var a billing.Account
	var archivedAt sql.NullTime
	var sellerID sql.NullInt64
	if err := sc.Scan(&a.ID, &a.TenantID, &a.TraccarUserID, &a.Name, &a.Email, &a.DeviceCount, &sellerID, &archivedAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return billing.Account{}, err
	}
	a.SellerID = sellerID.Int64
	a.ArchivedAt = scanTime(archivedAt)
	return a, nil
}

func (r *sqlRepository) UpsertAccount(ctx context.Context, a billing.Account) (billing.Account, error) {
	now := time.Now().UTC()

	var query string
	if r.dialect == "mysql" {
		query = `INSERT INTO accounts (tenant_id, traccar_user_id, name, email, device_count, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE name = VALUES(name), email = VALUES(email), device_count = VALUES(device_count), updated_at = VALUES(updated_at), archived_at = NULL`
	} else {
		query = `INSERT INTO accounts (tenant_id, traccar_user_id, name, email, device_count, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(tenant_id, traccar_user_id) DO UPDATE SET name = excluded.name, email = excluded.email, device_count = excluded.device_count, updated_at = excluded.updated_at, archived_at = NULL`
	}

	if _, err := r.q().ExecContext(ctx, query, a.TenantID, a.TraccarUserID, a.Name, a.Email, a.DeviceCount, now, now); err != nil {
		return billing.Account{}, fmt.Errorf("storage: upsert account: %w", err)
	}

	return r.scanAccount(r.q().QueryRowContext(ctx,
		`SELECT `+accountColumns+` FROM accounts WHERE tenant_id = ? AND traccar_user_id = ?`, a.TenantID, a.TraccarUserID))
}

func (r *sqlRepository) GetAccount(ctx context.Context, tenantID, accountID int64) (billing.Account, error) {
	return r.scanAccount(r.q().QueryRowContext(ctx,
		`SELECT `+accountColumns+` FROM accounts WHERE tenant_id = ? AND id = ?`, tenantID, accountID))
}

func (r *sqlRepository) scanAccount(row *sql.Row) (billing.Account, error) {
	a, err := scanAccountInto(row)
	if errors.Is(err, sql.ErrNoRows) {
		return billing.Account{}, billing.ErrNotFound
	}
	if err != nil {
		return billing.Account{}, fmt.Errorf("storage: scan account: %w", err)
	}
	return a, nil
}

func (r *sqlRepository) ListAccountsByTenant(ctx context.Context, tenantID int64) ([]billing.Account, error) {
	rows, err := r.q().QueryContext(ctx,
		`SELECT `+accountColumns+` FROM accounts WHERE tenant_id = ? AND archived_at IS NULL ORDER BY id`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("storage: list accounts: %w", err)
	}
	defer rows.Close()

	var accounts []billing.Account
	for rows.Next() {
		a, err := scanAccountInto(rows)
		if err != nil {
			return nil, fmt.Errorf("storage: scan account: %w", err)
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

func (r *sqlRepository) ArchiveAccount(ctx context.Context, accountID int64, archivedAt time.Time) error {
	if _, err := r.q().ExecContext(ctx,
		`UPDATE accounts SET archived_at = ?, updated_at = ? WHERE id = ? AND archived_at IS NULL`,
		archivedAt, time.Now().UTC(), accountID); err != nil {
		return fmt.Errorf("storage: archive account: %w", err)
	}
	return nil
}

const subscriptionColumns = `id, account_id, status, billing_mode, anchor_day, due_day, amount_cents, unit_price_cents, flat_fee_cents, min_devices, grace_days, currency, period_days, last_paid_at, next_due_at, created_at, updated_at`

const paymentColumns = `id, subscription_id, amount_cents, unit_price_cents, device_count, currency, method, reference, paid_at, note, voided_at, void_reason, created_at, updated_at`

type scanner interface {
	Scan(dest ...any) error
}

func scanSubscriptionInto(sc scanner) (billing.Subscription, error) {
	var s billing.Subscription
	var status, mode string
	var lastPaidAt sql.NullTime
	err := sc.Scan(&s.ID, &s.AccountID, &status, &mode, &s.AnchorDay, &s.DueDay, &s.AmountCents, &s.UnitPriceCents,
		&s.FlatFeeCents, &s.MinDevices, &s.GraceDays, &s.Currency, &s.PeriodDays, &lastPaidAt, &s.NextDueAt, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return billing.Subscription{}, err
	}
	s.Status = billing.SubscriptionStatus(status)
	s.BillingMode = billing.BillingMode(mode)
	s.LastPaidAt = scanTime(lastPaidAt)
	return s, nil
}

func scanPaymentInto(sc scanner, extra ...any) (billing.Payment, error) {
	var p billing.Payment
	var voidedAt, updatedAt sql.NullTime
	dest := []any{&p.ID, &p.SubscriptionID, &p.AmountCents, &p.UnitPriceCents, &p.DeviceCount, &p.Currency,
		&p.Method, &p.Reference, &p.PaidAt, &p.Note, &voidedAt, &p.VoidReason, &p.CreatedAt, &updatedAt}
	if err := sc.Scan(append(dest, extra...)...); err != nil {
		return billing.Payment{}, err
	}
	p.VoidedAt = scanTime(voidedAt)
	p.UpdatedAt = scanTime(updatedAt)
	return p, nil
}

func (r *sqlRepository) UpsertSubscription(ctx context.Context, s billing.Subscription) (billing.Subscription, error) {
	now := time.Now().UTC()

	if s.ID == 0 {
		res, err := r.q().ExecContext(ctx,
			`INSERT INTO subscriptions (account_id, status, billing_mode, anchor_day, due_day, amount_cents, unit_price_cents, flat_fee_cents, min_devices, grace_days, currency, period_days, last_paid_at, next_due_at, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			s.AccountID, string(s.Status), string(s.BillingMode), s.AnchorDay, s.DueDay, s.AmountCents, s.UnitPriceCents,
			s.FlatFeeCents, s.MinDevices, s.GraceDays, s.Currency, s.PeriodDays, nullTime(s.LastPaidAt), s.NextDueAt, now, now)
		if err != nil {
			return billing.Subscription{}, fmt.Errorf("storage: create subscription: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return billing.Subscription{}, fmt.Errorf("storage: create subscription: %w", err)
		}
		return r.GetSubscription(ctx, id)
	}

	if _, err := r.q().ExecContext(ctx,
		`UPDATE subscriptions SET status = ?, billing_mode = ?, anchor_day = ?, due_day = ?, amount_cents = ?, unit_price_cents = ?, flat_fee_cents = ?, min_devices = ?, grace_days = ?, currency = ?, period_days = ?, last_paid_at = ?, next_due_at = ?, updated_at = ?
		 WHERE id = ?`,
		string(s.Status), string(s.BillingMode), s.AnchorDay, s.DueDay, s.AmountCents, s.UnitPriceCents,
		s.FlatFeeCents, s.MinDevices, s.GraceDays, s.Currency, s.PeriodDays, nullTime(s.LastPaidAt), s.NextDueAt, now, s.ID); err != nil {
		return billing.Subscription{}, fmt.Errorf("storage: update subscription: %w", err)
	}
	return r.GetSubscription(ctx, s.ID)
}

func (r *sqlRepository) GetSubscription(ctx context.Context, id int64) (billing.Subscription, error) {
	return r.scanSubscription(r.q().QueryRowContext(ctx,
		`SELECT `+subscriptionColumns+` FROM subscriptions WHERE id = ?`, id))
}

func (r *sqlRepository) GetSubscriptionByAccountID(ctx context.Context, accountID int64) (billing.Subscription, error) {
	return r.scanSubscription(r.q().QueryRowContext(ctx,
		`SELECT `+subscriptionColumns+` FROM subscriptions WHERE account_id = ?`, accountID))
}

func (r *sqlRepository) scanSubscription(row *sql.Row) (billing.Subscription, error) {
	s, err := scanSubscriptionInto(row)
	if errors.Is(err, sql.ErrNoRows) {
		return billing.Subscription{}, billing.ErrNotFound
	}
	if err != nil {
		return billing.Subscription{}, fmt.Errorf("storage: scan subscription: %w", err)
	}
	return s, nil
}

func (r *sqlRepository) ListSubscriptionsDueBefore(ctx context.Context, tenantID int64, cutoff time.Time) ([]billing.Subscription, error) {
	rows, err := r.q().QueryContext(ctx,
		`SELECT s.id, s.account_id, s.status, s.billing_mode, s.anchor_day, s.due_day, s.amount_cents, s.unit_price_cents, s.flat_fee_cents, s.min_devices, s.grace_days, s.currency, s.period_days, s.last_paid_at, s.next_due_at, s.created_at, s.updated_at
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
		s, err := scanSubscriptionInto(rows)
		if err != nil {
			return nil, fmt.Errorf("storage: scan subscription: %w", err)
		}
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

func (r *sqlRepository) RecordPayment(ctx context.Context, p billing.Payment) (billing.Payment, error) {
	now := time.Now().UTC()
	res, err := r.q().ExecContext(ctx,
		`INSERT INTO payments (subscription_id, amount_cents, unit_price_cents, device_count, currency, method, reference, paid_at, note, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.SubscriptionID, p.AmountCents, p.UnitPriceCents, p.DeviceCount, p.Currency, p.Method, p.Reference, p.PaidAt, p.Note, now, now)
	if err != nil {
		return billing.Payment{}, fmt.Errorf("storage: record payment: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return billing.Payment{}, fmt.Errorf("storage: record payment: %w", err)
	}
	return r.getPaymentByID(ctx, id)
}

func (r *sqlRepository) getPaymentByID(ctx context.Context, id int64) (billing.Payment, error) {
	p, err := scanPaymentInto(r.q().QueryRowContext(ctx,
		`SELECT `+paymentColumns+` FROM payments WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return billing.Payment{}, billing.ErrNotFound
	}
	if err != nil {
		return billing.Payment{}, fmt.Errorf("storage: scan payment: %w", err)
	}
	return p, nil
}

func (r *sqlRepository) GetPayment(ctx context.Context, tenantID, paymentID int64) (billing.Payment, error) {
	p, err := scanPaymentInto(r.q().QueryRowContext(ctx,
		`SELECT p.id, p.subscription_id, p.amount_cents, p.unit_price_cents, p.device_count, p.currency, p.method, p.reference, p.paid_at, p.note, p.voided_at, p.void_reason, p.created_at, p.updated_at
		 FROM payments p
		 JOIN subscriptions s ON s.id = p.subscription_id
		 JOIN accounts a ON a.id = s.account_id
		 WHERE a.tenant_id = ? AND p.id = ?`, tenantID, paymentID))
	if errors.Is(err, sql.ErrNoRows) {
		return billing.Payment{}, billing.ErrNotFound
	}
	if err != nil {
		return billing.Payment{}, fmt.Errorf("storage: get payment: %w", err)
	}
	return p, nil
}

func (r *sqlRepository) UpdatePayment(ctx context.Context, p billing.Payment) (billing.Payment, error) {
	res, err := r.q().ExecContext(ctx,
		`UPDATE payments SET amount_cents = ?, unit_price_cents = ?, device_count = ?, currency = ?, method = ?, reference = ?, paid_at = ?, note = ?, updated_at = ?
		 WHERE id = ? AND voided_at IS NULL`,
		p.AmountCents, p.UnitPriceCents, p.DeviceCount, p.Currency, p.Method, p.Reference, p.PaidAt, p.Note, time.Now().UTC(), p.ID)
	if err != nil {
		return billing.Payment{}, fmt.Errorf("storage: update payment: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return billing.Payment{}, billing.ErrNotFound
	}
	return r.getPaymentByID(ctx, p.ID)
}

func (r *sqlRepository) VoidPayment(ctx context.Context, paymentID int64, voidedAt time.Time, reason string) error {
	res, err := r.q().ExecContext(ctx,
		`UPDATE payments SET voided_at = ?, void_reason = ?, updated_at = ? WHERE id = ? AND voided_at IS NULL`,
		voidedAt, reason, time.Now().UTC(), paymentID)
	if err != nil {
		return fmt.Errorf("storage: void payment: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return billing.ErrNotFound
	}
	return nil
}

func (r *sqlRepository) DeletePayment(ctx context.Context, tenantID, paymentID int64) error {
	if _, err := r.GetPayment(ctx, tenantID, paymentID); err != nil {
		return err
	}
	if _, err := r.q().ExecContext(ctx, `DELETE FROM payments WHERE id = ?`, paymentID); err != nil {
		return fmt.Errorf("storage: delete payment: %w", err)
	}
	return nil
}

func (r *sqlRepository) ListPaymentsBySubscription(ctx context.Context, subscriptionID int64) ([]billing.Payment, error) {
	rows, err := r.q().QueryContext(ctx,
		`SELECT `+paymentColumns+` FROM payments WHERE subscription_id = ? ORDER BY paid_at DESC, id DESC`, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("storage: list payments: %w", err)
	}
	defer rows.Close()

	var payments []billing.Payment
	for rows.Next() {
		p, err := scanPaymentInto(rows)
		if err != nil {
			return nil, fmt.Errorf("storage: scan payment: %w", err)
		}
		payments = append(payments, p)
	}
	return payments, rows.Err()
}

func (r *sqlRepository) ListPaymentsByTenant(ctx context.Context, tenantID int64) ([]billing.TenantPayment, error) {
	rows, err := r.q().QueryContext(ctx,
		`SELECT p.id, p.subscription_id, p.amount_cents, p.unit_price_cents, p.device_count, p.currency, p.method, p.reference, p.paid_at, p.note, p.voided_at, p.void_reason, p.created_at, p.updated_at, a.id, a.name
		 FROM payments p
		 JOIN subscriptions s ON s.id = p.subscription_id
		 JOIN accounts a ON a.id = s.account_id
		 WHERE a.tenant_id = ?
		 ORDER BY p.paid_at DESC, p.id DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("storage: list tenant payments: %w", err)
	}
	defer rows.Close()

	var payments []billing.TenantPayment
	for rows.Next() {
		var tp billing.TenantPayment
		p, err := scanPaymentInto(rows, &tp.AccountID, &tp.AccountName)
		if err != nil {
			return nil, fmt.Errorf("storage: scan tenant payment: %w", err)
		}
		tp.Payment = p
		payments = append(payments, tp)
	}
	return payments, rows.Err()
}

const sellerColumns = `id, tenant_id, name, email, phone, commission_bp, active, note, created_at, updated_at`

func scanSellerInto(sc scanner) (billing.Seller, error) {
	var s billing.Seller
	var active int
	if err := sc.Scan(&s.ID, &s.TenantID, &s.Name, &s.Email, &s.Phone, &s.CommissionBP, &active, &s.Note, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return billing.Seller{}, err
	}
	s.Active = active != 0
	return s, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (r *sqlRepository) CreateSeller(ctx context.Context, s billing.Seller) (billing.Seller, error) {
	now := time.Now().UTC()
	res, err := r.q().ExecContext(ctx,
		`INSERT INTO sellers (tenant_id, name, email, phone, commission_bp, active, note, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.TenantID, s.Name, s.Email, s.Phone, s.CommissionBP, boolToInt(s.Active), s.Note, now, now)
	if err != nil {
		return billing.Seller{}, fmt.Errorf("storage: create seller: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return billing.Seller{}, fmt.Errorf("storage: create seller: %w", err)
	}
	return r.GetSeller(ctx, s.TenantID, id)
}

func (r *sqlRepository) UpdateSeller(ctx context.Context, s billing.Seller) (billing.Seller, error) {
	res, err := r.q().ExecContext(ctx,
		`UPDATE sellers SET name = ?, email = ?, phone = ?, commission_bp = ?, active = ?, note = ?, updated_at = ?
		 WHERE id = ? AND tenant_id = ?`,
		s.Name, s.Email, s.Phone, s.CommissionBP, boolToInt(s.Active), s.Note, time.Now().UTC(), s.ID, s.TenantID)
	if err != nil {
		return billing.Seller{}, fmt.Errorf("storage: update seller: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return billing.Seller{}, billing.ErrNotFound
	}
	return r.GetSeller(ctx, s.TenantID, s.ID)
}

func (r *sqlRepository) GetSeller(ctx context.Context, tenantID, sellerID int64) (billing.Seller, error) {
	s, err := scanSellerInto(r.q().QueryRowContext(ctx,
		`SELECT `+sellerColumns+` FROM sellers WHERE tenant_id = ? AND id = ?`, tenantID, sellerID))
	if errors.Is(err, sql.ErrNoRows) {
		return billing.Seller{}, billing.ErrNotFound
	}
	if err != nil {
		return billing.Seller{}, fmt.Errorf("storage: get seller: %w", err)
	}
	return s, nil
}

func (r *sqlRepository) ListSellers(ctx context.Context, tenantID int64) ([]billing.Seller, error) {
	rows, err := r.q().QueryContext(ctx,
		`SELECT `+sellerColumns+` FROM sellers WHERE tenant_id = ? ORDER BY active DESC, name`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("storage: list sellers: %w", err)
	}
	defer rows.Close()

	var sellers []billing.Seller
	for rows.Next() {
		s, err := scanSellerInto(rows)
		if err != nil {
			return nil, fmt.Errorf("storage: scan seller: %w", err)
		}
		sellers = append(sellers, s)
	}
	return sellers, rows.Err()
}

func (r *sqlRepository) AssignAccountSeller(ctx context.Context, tenantID, accountID, sellerID int64) error {
	var seller any
	if sellerID > 0 {
		if _, err := r.GetSeller(ctx, tenantID, sellerID); err != nil {
			return err
		}
		seller = sellerID
	}

	res, err := r.q().ExecContext(ctx,
		`UPDATE accounts SET seller_id = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
		seller, time.Now().UTC(), accountID, tenantID)
	if err != nil {
		return fmt.Errorf("storage: assign account seller: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return billing.ErrNotFound
	}
	return nil
}
