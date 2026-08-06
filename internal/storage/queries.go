package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"modernc.org/sqlite"

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

func nullInt64(id int64) any {
	if id <= 0 {
		return nil
	}
	return id
}

func scanTime(nt sql.NullTime) time.Time {
	if !nt.Valid {
		return time.Time{}
	}
	return nt.Time
}

const tenantColumns = `id, name, base_url, traccar_user_id, owner_email, session_cookie, api_token, connectivity_provider, connectivity_token, session_expires_at, admin_traccar_user_id, created_at, updated_at`

func scanTenantInto(sc scanner) (billing.Tenant, error) {
	var t billing.Tenant
	var sessionExpiresAt sql.NullTime
	if err := sc.Scan(&t.ID, &t.Name, &t.BaseURL, &t.TraccarUserID, &t.OwnerEmail, &t.SessionCookie, &t.APIToken, &t.ConnectivityProvider, &t.ConnectivityToken, &sessionExpiresAt, &t.AdminTraccarUserID, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return billing.Tenant{}, err
	}
	t.SessionExpiresAt = scanTime(sessionExpiresAt)
	return t, nil
}

func (r *sqlRepository) CreateTenant(ctx context.Context, t billing.Tenant) (billing.Tenant, error) {
	if r.tx == nil {
		var created billing.Tenant
		err := r.WithTx(ctx, func(tx billing.Repository) error {
			var err error
			created, err = tx.CreateTenant(ctx, t)
			return err
		})
		return created, err
	}

	now := time.Now().UTC()
	res, err := r.q().ExecContext(ctx,
		`INSERT INTO tenants (name, base_url, traccar_user_id, owner_email, session_cookie, api_token, session_expires_at, admin_traccar_user_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.Name, t.BaseURL, t.TraccarUserID, t.OwnerEmail, t.SessionCookie, t.APIToken, nullTime(t.SessionExpiresAt), t.AdminTraccarUserID, now, now)
	if err != nil {
		return billing.Tenant{}, fmt.Errorf("storage: create tenant: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return billing.Tenant{}, fmt.Errorf("storage: create tenant: %w", err)
	}

	// Seed the same catalog migration 000009 gave the existing tenants, so
	// a tenant created later still opens with the three usual concepts.
	if _, err := r.q().ExecContext(ctx,
		`INSERT INTO concepts (tenant_id, name, slug, amount_cents, currency, recurring, active, note, created_at, updated_at)
		 VALUES
		 (?, 'Instalación', 'instalacion', 0, 'MXN', 0, 1, '', ?, ?),
		 (?, 'Mensualidad', 'mensualidad', 0, 'MXN', 1, 1, '', ?, ?),
		 (?, 'Desinstalación', 'desinstalacion', 0, 'MXN', 0, 1, '', ?, ?)`,
		id, now, now, id, now, now, id, now, now); err != nil {
		return billing.Tenant{}, fmt.Errorf("storage: seed tenant concepts: %w", err)
	}

	return r.GetTenantByID(ctx, id)
}

func (r *sqlRepository) GetTenantByID(ctx context.Context, id int64) (billing.Tenant, error) {
	return r.scanTenant(r.q().QueryRowContext(ctx,
		`SELECT `+tenantColumns+` FROM tenants WHERE id = ?`, id))
}

func (r *sqlRepository) GetTenantByOwner(ctx context.Context, baseURL string, traccarUserID int64) (billing.Tenant, error) {
	return r.scanTenant(r.q().QueryRowContext(ctx,
		`SELECT `+tenantColumns+` FROM tenants WHERE base_url = ? AND traccar_user_id = ?`, baseURL, traccarUserID))
}

func (r *sqlRepository) scanTenant(row *sql.Row) (billing.Tenant, error) {
	t, err := scanTenantInto(row)
	if errors.Is(err, sql.ErrNoRows) {
		return billing.Tenant{}, billing.ErrNotFound
	}
	if err != nil {
		return billing.Tenant{}, fmt.Errorf("storage: scan tenant: %w", err)
	}
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

func (r *sqlRepository) UpdateTenantOwner(ctx context.Context, tenantID int64, traccarUserID int64, email string) error {
	_, err := r.q().ExecContext(ctx,
		`UPDATE tenants SET admin_traccar_user_id = ?, owner_email = ?, updated_at = ? WHERE id = ?`,
		traccarUserID, email, time.Now().UTC(), tenantID)
	if err != nil {
		return fmt.Errorf("storage: update tenant owner: %w", err)
	}
	return nil
}

func (r *sqlRepository) UpdateTenantAPIToken(ctx context.Context, tenantID int64, token string) error {
	_, err := r.q().ExecContext(ctx,
		`UPDATE tenants SET api_token = ?, updated_at = ? WHERE id = ?`,
		token, time.Now().UTC(), tenantID)
	if err != nil {
		return fmt.Errorf("storage: update tenant api token: %w", err)
	}
	return nil
}

func (r *sqlRepository) UpdateTenantConnectivity(ctx context.Context, tenantID int64, providerID, encryptedToken string) error {
	_, err := r.q().ExecContext(ctx,
		`UPDATE tenants SET connectivity_provider = ?, connectivity_token = ?, updated_at = ? WHERE id = ?`,
		providerID, encryptedToken, time.Now().UTC(), tenantID)
	if err != nil {
		return fmt.Errorf("storage: update tenant connectivity: %w", err)
	}
	return nil
}

func (r *sqlRepository) GetSIMInventoryCache(ctx context.Context, tenantID int64) (string, time.Time, bool, error) {
	var payload string
	var refreshedAt time.Time
	err := r.q().QueryRowContext(ctx,
		`SELECT payload, refreshed_at FROM sim_inventory_cache WHERE tenant_id = ?`, tenantID).
		Scan(&payload, &refreshedAt)
	if err == sql.ErrNoRows {
		return "", time.Time{}, false, nil
	}
	if err != nil {
		return "", time.Time{}, false, fmt.Errorf("storage: get sim inventory cache: %w", err)
	}
	return payload, refreshedAt, true, nil
}

func (r *sqlRepository) SaveSIMInventoryCache(ctx context.Context, tenantID int64, payload string, refreshedAt time.Time) error {
	refreshedAt = refreshedAt.UTC()

	var query string
	if r.dialect == "mysql" {
		query = `INSERT INTO sim_inventory_cache (tenant_id, payload, refreshed_at)
			VALUES (?, ?, ?)
			ON DUPLICATE KEY UPDATE payload = VALUES(payload), refreshed_at = VALUES(refreshed_at)`
	} else {
		query = `INSERT INTO sim_inventory_cache (tenant_id, payload, refreshed_at)
			VALUES (?, ?, ?)
			ON CONFLICT(tenant_id) DO UPDATE SET payload = excluded.payload, refreshed_at = excluded.refreshed_at`
	}

	if _, err := r.q().ExecContext(ctx, query, tenantID, payload, refreshedAt); err != nil {
		return fmt.Errorf("storage: save sim inventory cache: %w", err)
	}
	return nil
}

func (r *sqlRepository) DeleteSIMInventoryCache(ctx context.Context, tenantID int64) error {
	if _, err := r.q().ExecContext(ctx, `DELETE FROM sim_inventory_cache WHERE tenant_id = ?`, tenantID); err != nil {
		return fmt.Errorf("storage: delete sim inventory cache: %w", err)
	}
	return nil
}

func (r *sqlRepository) ListTenants(ctx context.Context) ([]billing.Tenant, error) {
	rows, err := r.q().QueryContext(ctx,
		`SELECT `+tenantColumns+` FROM tenants ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("storage: list tenants: %w", err)
	}
	defer rows.Close()

	var tenants []billing.Tenant
	for rows.Next() {
		t, err := scanTenantInto(rows)
		if err != nil {
			return nil, fmt.Errorf("storage: scan tenant: %w", err)
		}
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

// DeleteAccount cascades by hand: the schema declares the account ->
// subscription -> payment foreign keys without ON DELETE CASCADE, and
// SQLite only enforces them when foreign_keys pragma is on, so relying on
// the engine would behave differently per dialect.
func (r *sqlRepository) DeleteAccount(ctx context.Context, tenantID, accountID int64) error {
	if _, err := r.GetAccount(ctx, tenantID, accountID); err != nil {
		return err
	}

	if _, err := r.q().ExecContext(ctx,
		`DELETE FROM payment_items WHERE payment_id IN (SELECT id FROM payments WHERE account_id = ?)`,
		accountID); err != nil {
		return fmt.Errorf("storage: delete account payment items: %w", err)
	}
	if _, err := r.q().ExecContext(ctx, `DELETE FROM payments WHERE account_id = ?`, accountID); err != nil {
		return fmt.Errorf("storage: delete account payments: %w", err)
	}
	if _, err := r.q().ExecContext(ctx, `DELETE FROM subscriptions WHERE account_id = ?`, accountID); err != nil {
		return fmt.Errorf("storage: delete account subscription: %w", err)
	}
	if _, err := r.q().ExecContext(ctx, `DELETE FROM accounts WHERE id = ? AND tenant_id = ?`, accountID, tenantID); err != nil {
		return fmt.Errorf("storage: delete account: %w", err)
	}
	return nil
}

const subscriptionColumns = `id, account_id, status, billing_mode, anchor_day, due_day, amount_cents, unit_price_cents, flat_fee_cents, min_devices, grace_days, currency, period_days, last_paid_at, next_due_at, created_at, updated_at`

const paymentColumns = `id, account_id, subscription_id, concept_id, amount_cents, unit_price_cents, device_count, currency, method, reference, paid_at, note, voided_at, void_reason, created_at, updated_at`

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
	var subscriptionID, conceptID sql.NullInt64
	var voidedAt, updatedAt sql.NullTime
	dest := []any{&p.ID, &p.AccountID, &subscriptionID, &conceptID, &p.AmountCents, &p.UnitPriceCents, &p.DeviceCount, &p.Currency,
		&p.Method, &p.Reference, &p.PaidAt, &p.Note, &voidedAt, &p.VoidReason, &p.CreatedAt, &updatedAt}
	if err := sc.Scan(append(dest, extra...)...); err != nil {
		return billing.Payment{}, err
	}
	p.SubscriptionID = subscriptionID.Int64
	p.ConceptID = conceptID.Int64
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
	p, err := r.resolvePaymentAccount(ctx, p)
	if err != nil {
		return billing.Payment{}, err
	}
	now := time.Now().UTC()
	res, err := r.q().ExecContext(ctx,
		`INSERT INTO payments (account_id, subscription_id, concept_id, amount_cents, unit_price_cents, device_count, currency, method, reference, paid_at, note, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.AccountID, nullInt64(p.SubscriptionID), nullInt64(p.ConceptID), p.AmountCents, p.UnitPriceCents, p.DeviceCount, p.Currency, p.Method, p.Reference, p.PaidAt, p.Note, now, now)
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
		`SELECT p.id, p.account_id, p.subscription_id, p.concept_id, p.amount_cents, p.unit_price_cents, p.device_count, p.currency, p.method, p.reference, p.paid_at, p.note, p.voided_at, p.void_reason, p.created_at, p.updated_at
		 FROM payments p
		 JOIN accounts a ON a.id = p.account_id
		 WHERE a.tenant_id = ? AND p.id = ?`, tenantID, paymentID))
	if errors.Is(err, sql.ErrNoRows) {
		return billing.Payment{}, billing.ErrNotFound
	}
	if err != nil {
		return billing.Payment{}, fmt.Errorf("storage: get payment: %w", err)
	}
	return p, nil
}

// resolvePaymentAccount fills AccountID from the subscription when the
// caller only knows the subscription, which is how every pre-000010 caller
// still builds a payment. account_id is NOT NULL, so leaving it at zero
// would fail on the insert with an opaque constraint error instead.
func (r *sqlRepository) resolvePaymentAccount(ctx context.Context, p billing.Payment) (billing.Payment, error) {
	if p.AccountID > 0 || p.SubscriptionID == 0 {
		return p, nil
	}
	if err := r.q().QueryRowContext(ctx,
		`SELECT account_id FROM subscriptions WHERE id = ?`, p.SubscriptionID).Scan(&p.AccountID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return billing.Payment{}, billing.ErrNotFound
		}
		return billing.Payment{}, fmt.Errorf("storage: resolve payment account: %w", err)
	}
	return p, nil
}

func (r *sqlRepository) UpdatePayment(ctx context.Context, p billing.Payment) (billing.Payment, error) {
	p, err := r.resolvePaymentAccount(ctx, p)
	if err != nil {
		return billing.Payment{}, err
	}
	res, err := r.q().ExecContext(ctx,
		`UPDATE payments SET account_id = ?, subscription_id = ?, concept_id = ?, amount_cents = ?, unit_price_cents = ?, device_count = ?, currency = ?, method = ?, reference = ?, paid_at = ?, note = ?, updated_at = ?
		 WHERE id = ? AND voided_at IS NULL`,
		p.AccountID, nullInt64(p.SubscriptionID), nullInt64(p.ConceptID), p.AmountCents, p.UnitPriceCents, p.DeviceCount, p.Currency, p.Method, p.Reference, p.PaidAt, p.Note, time.Now().UTC(), p.ID)
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

func (r *sqlRepository) ListPaymentsByTenant(ctx context.Context, tenantID int64, filter billing.PaymentFilter) ([]billing.TenantPayment, error) {
	query := `SELECT p.id, p.account_id, p.subscription_id, p.concept_id, p.amount_cents, p.unit_price_cents, p.device_count, p.currency, p.method, p.reference, p.paid_at, p.note, p.voided_at, p.void_reason, p.created_at, p.updated_at, a.name, COALESCE(c.name, ''), COALESCE(c.recurring, 1)
		 FROM payments p
		 JOIN accounts a ON a.id = p.account_id
		 LEFT JOIN concepts c ON c.id = p.concept_id
		 WHERE a.tenant_id = ?`
	args := []any{tenantID}

	if !filter.From.IsZero() {
		query += ` AND p.paid_at >= ?`
		args = append(args, filter.From.UTC())
	}
	if !filter.To.IsZero() {
		query += ` AND p.paid_at < ?`
		args = append(args, filter.To.UTC())
	}
	if filter.AccountID > 0 {
		query += ` AND p.account_id = ?`
		args = append(args, filter.AccountID)
	}
	query += ` ORDER BY p.paid_at DESC, p.id DESC`

	rows, err := r.q().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("storage: list tenant payments: %w", err)
	}
	defer rows.Close()

	var payments []billing.TenantPayment
	for rows.Next() {
		var tp billing.TenantPayment
		var conceptRecurring int
		p, err := scanPaymentInto(rows, &tp.AccountName, &tp.ConceptName, &conceptRecurring)
		if err != nil {
			return nil, fmt.Errorf("storage: scan tenant payment: %w", err)
		}
		tp.Payment = p
		tp.ConceptRecurring = conceptRecurring != 0
		payments = append(payments, tp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: list tenant payments: %w", err)
	}

	ids := make([]int64, len(payments))
	for i, p := range payments {
		ids[i] = p.ID
	}
	itemsByPayment, err := r.paymentItemsFor(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range payments {
		payments[i].Items = itemsByPayment[payments[i].ID]
	}
	return payments, nil
}

// paymentItemsFor loads the lines of a whole page of payments in one query, so
// listing them does not turn into a query per row.
func (r *sqlRepository) paymentItemsFor(ctx context.Context, paymentIDs []int64) (map[int64][]billing.PaymentItem, error) {
	if len(paymentIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(paymentIDs)), ",")
	args := make([]any, len(paymentIDs))
	for i, id := range paymentIDs {
		args[i] = id
	}

	rows, err := r.q().QueryContext(ctx,
		`SELECT `+paymentItemColumns+` FROM payment_items WHERE payment_id IN (`+placeholders+`) ORDER BY payment_id ASC, position ASC, id ASC`, args...)
	if err != nil {
		return nil, fmt.Errorf("storage: list payment items: %w", err)
	}
	defer rows.Close()

	byPayment := make(map[int64][]billing.PaymentItem)
	for rows.Next() {
		item, err := scanPaymentItemInto(rows)
		if err != nil {
			return nil, fmt.Errorf("storage: scan payment item: %w", err)
		}
		byPayment[item.PaymentID] = append(byPayment[item.PaymentID], item)
	}
	return byPayment, rows.Err()
}

const settingsColumns = `tenant_id, billing_mode, anchor_day, due_day, period_days, grace_days, currency, unit_price_cents, flat_fee_cents, min_devices, hide_mirror_accounts, created_at, updated_at`

func (r *sqlRepository) GetSettings(ctx context.Context, tenantID int64) (billing.Settings, error) {
	var s billing.Settings
	var mode string
	var hideMirror int

	err := r.q().QueryRowContext(ctx,
		`SELECT `+settingsColumns+` FROM tenant_settings WHERE tenant_id = ?`, tenantID).
		Scan(&s.TenantID, &mode, &s.AnchorDay, &s.DueDay, &s.PeriodDays, &s.GraceDays, &s.Currency,
			&s.UnitPriceCents, &s.FlatFeeCents, &s.MinDevices, &hideMirror, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return billing.DefaultSettings(tenantID), nil
	}
	if err != nil {
		return billing.Settings{}, fmt.Errorf("storage: get settings: %w", err)
	}

	s.BillingMode = billing.BillingMode(mode)
	s.HideMirrorAccounts = hideMirror != 0
	return s.Normalized(), nil
}

func (r *sqlRepository) SaveSettings(ctx context.Context, s billing.Settings) (billing.Settings, error) {
	s = s.Normalized()
	now := time.Now().UTC()

	var query string
	if r.dialect == "mysql" {
		query = `INSERT INTO tenant_settings (` + settingsColumns + `)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE billing_mode = VALUES(billing_mode), anchor_day = VALUES(anchor_day),
				due_day = VALUES(due_day), period_days = VALUES(period_days), grace_days = VALUES(grace_days),
				currency = VALUES(currency), unit_price_cents = VALUES(unit_price_cents),
				flat_fee_cents = VALUES(flat_fee_cents), min_devices = VALUES(min_devices),
				hide_mirror_accounts = VALUES(hide_mirror_accounts), updated_at = VALUES(updated_at)`
	} else {
		query = `INSERT INTO tenant_settings (` + settingsColumns + `)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(tenant_id) DO UPDATE SET billing_mode = excluded.billing_mode, anchor_day = excluded.anchor_day,
				due_day = excluded.due_day, period_days = excluded.period_days, grace_days = excluded.grace_days,
				currency = excluded.currency, unit_price_cents = excluded.unit_price_cents,
				flat_fee_cents = excluded.flat_fee_cents, min_devices = excluded.min_devices,
				hide_mirror_accounts = excluded.hide_mirror_accounts, updated_at = excluded.updated_at`
	}

	if _, err := r.q().ExecContext(ctx, query,
		s.TenantID, string(s.BillingMode), s.AnchorDay, s.DueDay, s.PeriodDays, s.GraceDays, s.Currency,
		s.UnitPriceCents, s.FlatFeeCents, s.MinDevices, boolToInt(s.HideMirrorAccounts), now, now); err != nil {
		return billing.Settings{}, fmt.Errorf("storage: save settings: %w", err)
	}
	return r.GetSettings(ctx, s.TenantID)
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

const conceptColumns = `id, tenant_id, name, slug, amount_cents, currency, recurring, active, note, created_at, updated_at`

func scanConceptInto(sc scanner) (billing.Concept, error) {
	var c billing.Concept
	var recurring, active int
	if err := sc.Scan(&c.ID, &c.TenantID, &c.Name, &c.Slug, &c.AmountCents, &c.Currency, &recurring, &active, &c.Note, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return billing.Concept{}, err
	}
	c.Recurring = recurring != 0
	c.Active = active != 0
	return c, nil
}

func (r *sqlRepository) CreateConcept(ctx context.Context, c billing.Concept) (billing.Concept, error) {
	c = c.Normalized()
	now := time.Now().UTC()
	res, err := r.q().ExecContext(ctx,
		`INSERT INTO concepts (tenant_id, name, slug, amount_cents, currency, recurring, active, note, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.TenantID, c.Name, c.Slug, c.AmountCents, c.Currency, boolToInt(c.Recurring), boolToInt(c.Active), c.Note, now, now)
	if err != nil {
		return billing.Concept{}, fmt.Errorf("storage: create concept: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return billing.Concept{}, fmt.Errorf("storage: create concept: %w", err)
	}
	return r.GetConcept(ctx, c.TenantID, id)
}

func (r *sqlRepository) UpdateConcept(ctx context.Context, c billing.Concept) (billing.Concept, error) {
	c = c.Normalized()
	res, err := r.q().ExecContext(ctx,
		`UPDATE concepts SET name = ?, slug = ?, amount_cents = ?, currency = ?, recurring = ?, active = ?, note = ?, updated_at = ?
		 WHERE id = ? AND tenant_id = ?`,
		c.Name, c.Slug, c.AmountCents, c.Currency, boolToInt(c.Recurring), boolToInt(c.Active), c.Note, time.Now().UTC(), c.ID, c.TenantID)
	if err != nil {
		return billing.Concept{}, fmt.Errorf("storage: update concept: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return billing.Concept{}, billing.ErrNotFound
	}
	return r.GetConcept(ctx, c.TenantID, c.ID)
}

func (r *sqlRepository) GetConcept(ctx context.Context, tenantID, conceptID int64) (billing.Concept, error) {
	c, err := scanConceptInto(r.q().QueryRowContext(ctx,
		`SELECT `+conceptColumns+` FROM concepts WHERE tenant_id = ? AND id = ?`, tenantID, conceptID))
	if errors.Is(err, sql.ErrNoRows) {
		return billing.Concept{}, billing.ErrNotFound
	}
	if err != nil {
		return billing.Concept{}, fmt.Errorf("storage: get concept: %w", err)
	}
	return c, nil
}

func (r *sqlRepository) ListConcepts(ctx context.Context, tenantID int64) ([]billing.Concept, error) {
	rows, err := r.q().QueryContext(ctx,
		`SELECT `+conceptColumns+` FROM concepts WHERE tenant_id = ? ORDER BY active DESC, name`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("storage: list concepts: %w", err)
	}
	defer rows.Close()

	var concepts []billing.Concept
	for rows.Next() {
		c, err := scanConceptInto(rows)
		if err != nil {
			return nil, fmt.Errorf("storage: scan concept: %w", err)
		}
		concepts = append(concepts, c)
	}
	return concepts, rows.Err()
}

func (r *sqlRepository) DeleteConcept(ctx context.Context, tenantID, conceptID int64) error {
	if _, err := r.GetConcept(ctx, tenantID, conceptID); err != nil {
		return err
	}

	var count int
	if err := r.q().QueryRowContext(ctx, `SELECT COUNT(*) FROM payments WHERE concept_id = ?`, conceptID).Scan(&count); err != nil {
		return fmt.Errorf("storage: check concept payments: %w", err)
	}

	now := time.Now().UTC()
	if count > 0 {
		_, err := r.q().ExecContext(ctx, `UPDATE concepts SET active = 0, updated_at = ? WHERE id = ? AND tenant_id = ?`, now, conceptID, tenantID)
		if err != nil {
			return fmt.Errorf("storage: deactivate concept: %w", err)
		}
		return nil
	}

	res, err := r.q().ExecContext(ctx, `DELETE FROM concepts WHERE id = ? AND tenant_id = ?`, conceptID, tenantID)
	if err != nil {
		return fmt.Errorf("storage: delete concept: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return billing.ErrNotFound
	}
	return nil
}

const paymentItemColumns = `id, payment_id, concept_id, description, quantity, unit_price_cents, amount_cents, position, created_at`

func scanPaymentItemInto(sc scanner) (billing.PaymentItem, error) {
	var item billing.PaymentItem
	var conceptID sql.NullInt64
	if err := sc.Scan(&item.ID, &item.PaymentID, &conceptID, &item.Description, &item.Quantity, &item.UnitPriceCents, &item.AmountCents, &item.Position, &item.CreatedAt); err != nil {
		return billing.PaymentItem{}, err
	}
	item.ConceptID = conceptID.Int64
	return item, nil
}

func (r *sqlRepository) ListPaymentItems(ctx context.Context, paymentID int64) ([]billing.PaymentItem, error) {
	rows, err := r.q().QueryContext(ctx,
		`SELECT `+paymentItemColumns+` FROM payment_items WHERE payment_id = ? ORDER BY position ASC, id ASC`, paymentID)
	if err != nil {
		return nil, fmt.Errorf("storage: list payment items: %w", err)
	}
	defer rows.Close()

	var items []billing.PaymentItem
	for rows.Next() {
		item, err := scanPaymentItemInto(rows)
		if err != nil {
			return nil, fmt.Errorf("storage: scan payment item: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *sqlRepository) ReplacePaymentItems(ctx context.Context, paymentID int64, items []billing.PaymentItem) ([]billing.PaymentItem, error) {
	var result []billing.PaymentItem
	err := r.WithTx(ctx, func(txRepo billing.Repository) error {
		tx, ok := txRepo.(*sqlRepository)
		if !ok {
			return fmt.Errorf("storage: unexpected repository type %T", txRepo)
		}
		if _, err := tx.q().ExecContext(ctx, `DELETE FROM payment_items WHERE payment_id = ?`, paymentID); err != nil {
			return fmt.Errorf("storage: delete payment items: %w", err)
		}

		now := time.Now().UTC()
		result = make([]billing.PaymentItem, 0, len(items))
		for pos, item := range items {
			item.PaymentID = paymentID
			item.Position = pos
			res, err := tx.q().ExecContext(ctx,
				`INSERT INTO payment_items (payment_id, concept_id, description, quantity, unit_price_cents, amount_cents, position, created_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				paymentID, nullInt64(item.ConceptID), item.Description, item.Quantity, item.UnitPriceCents, item.AmountCents, item.Position, now)
			if err != nil {
				return fmt.Errorf("storage: create payment item: %w", err)
			}
			id, err := res.LastInsertId()
			if err != nil {
				return fmt.Errorf("storage: create payment item: %w", err)
			}
			item.ID = id
			item.CreatedAt = now
			result = append(result, item)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

const expenseColumns = `id, tenant_id, seller_id, category, amount_cents, currency, spent_at, method, reference, note, created_at, updated_at`

func scanExpenseInto(sc scanner) (billing.Expense, error) {
	var e billing.Expense
	var sellerID sql.NullInt64
	if err := sc.Scan(&e.ID, &e.TenantID, &sellerID, &e.Category, &e.AmountCents, &e.Currency, &e.SpentAt, &e.Method, &e.Reference, &e.Note, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return billing.Expense{}, err
	}
	e.SellerID = sellerID.Int64
	return e, nil
}

func (r *sqlRepository) CreateExpense(ctx context.Context, e billing.Expense) (billing.Expense, error) {
	e = e.Normalized()
	now := time.Now().UTC()
	res, err := r.q().ExecContext(ctx,
		`INSERT INTO expenses (tenant_id, seller_id, category, amount_cents, currency, spent_at, method, reference, note, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.TenantID, nullInt64(e.SellerID), e.Category, e.AmountCents, e.Currency, e.SpentAt, e.Method, e.Reference, e.Note, now, now)
	if err != nil {
		return billing.Expense{}, fmt.Errorf("storage: create expense: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return billing.Expense{}, fmt.Errorf("storage: create expense: %w", err)
	}
	return r.GetExpense(ctx, e.TenantID, id)
}

func (r *sqlRepository) UpdateExpense(ctx context.Context, e billing.Expense) (billing.Expense, error) {
	e = e.Normalized()
	now := time.Now().UTC()
	res, err := r.q().ExecContext(ctx,
		`UPDATE expenses SET seller_id = ?, category = ?, amount_cents = ?, currency = ?, spent_at = ?, method = ?, reference = ?, note = ?, updated_at = ?
		 WHERE id = ? AND tenant_id = ?`,
		nullInt64(e.SellerID), e.Category, e.AmountCents, e.Currency, e.SpentAt, e.Method, e.Reference, e.Note, now, e.ID, e.TenantID)
	if err != nil {
		return billing.Expense{}, fmt.Errorf("storage: update expense: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return billing.Expense{}, billing.ErrNotFound
	}
	return r.GetExpense(ctx, e.TenantID, e.ID)
}

func (r *sqlRepository) GetExpense(ctx context.Context, tenantID, expenseID int64) (billing.Expense, error) {
	e, err := scanExpenseInto(r.q().QueryRowContext(ctx,
		`SELECT `+expenseColumns+` FROM expenses WHERE tenant_id = ? AND id = ?`, tenantID, expenseID))
	if errors.Is(err, sql.ErrNoRows) {
		return billing.Expense{}, billing.ErrNotFound
	}
	if err != nil {
		return billing.Expense{}, fmt.Errorf("storage: get expense: %w", err)
	}
	return e, nil
}

func (r *sqlRepository) ListExpenses(ctx context.Context, tenantID int64, filter billing.PaymentFilter) ([]billing.Expense, error) {
	query := `SELECT ` + expenseColumns + ` FROM expenses WHERE tenant_id = ?`
	args := []any{tenantID}

	if !filter.From.IsZero() {
		query += ` AND spent_at >= ?`
		args = append(args, filter.From.UTC())
	}
	if !filter.To.IsZero() {
		query += ` AND spent_at < ?`
		args = append(args, filter.To.UTC())
	}
	query += ` ORDER BY spent_at DESC, id DESC`

	rows, err := r.q().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("storage: list expenses: %w", err)
	}
	defer rows.Close()

	var expenses []billing.Expense
	for rows.Next() {
		e, err := scanExpenseInto(rows)
		if err != nil {
			return nil, fmt.Errorf("storage: scan expense: %w", err)
		}
		expenses = append(expenses, e)
	}
	return expenses, rows.Err()
}

func (r *sqlRepository) DeleteExpense(ctx context.Context, tenantID, expenseID int64) error {
	res, err := r.q().ExecContext(ctx, `DELETE FROM expenses WHERE id = ? AND tenant_id = ?`, expenseID, tenantID)
	if err != nil {
		return fmt.Errorf("storage: delete expense: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return billing.ErrNotFound
	}
	return nil
}

const appointmentColumns = `id, tenant_id, account_id, seller_id, client_name, phone, unit, address, device_count, amount_cents, currency, scheduled_on, time_window, status, note, outcome, closed_at, created_at, updated_at`

func scanAppointmentInto(sc scanner) (billing.Appointment, error) {
	var a billing.Appointment
	var accountID, sellerID sql.NullInt64
	var closedAt sql.NullTime
	var status string
	if err := sc.Scan(&a.ID, &a.TenantID, &accountID, &sellerID, &a.ClientName, &a.Phone, &a.Unit, &a.Address,
		&a.DeviceCount, &a.AmountCents, &a.Currency, &a.ScheduledOn, &a.TimeWindow, &status, &a.Note, &a.Outcome,
		&closedAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return billing.Appointment{}, err
	}
	a.AccountID = accountID.Int64
	a.SellerID = sellerID.Int64
	a.ClosedAt = closedAt.Time
	a.Status = billing.AppointmentStatus(status)
	return a, nil
}

func (r *sqlRepository) CreateAppointment(ctx context.Context, a billing.Appointment) (billing.Appointment, error) {
	a = a.Normalized()
	now := time.Now().UTC()
	res, err := r.q().ExecContext(ctx,
		`INSERT INTO appointments (tenant_id, account_id, seller_id, client_name, phone, unit, address, device_count, amount_cents, currency, scheduled_on, time_window, status, note, outcome, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.TenantID, nullInt64(a.AccountID), nullInt64(a.SellerID), a.ClientName, a.Phone, a.Unit, a.Address,
		a.DeviceCount, a.AmountCents, a.Currency, a.ScheduledOn, a.TimeWindow, string(a.Status), a.Note, a.Outcome, now, now)
	if err != nil {
		return billing.Appointment{}, fmt.Errorf("storage: create appointment: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return billing.Appointment{}, fmt.Errorf("storage: create appointment: %w", err)
	}
	return r.GetAppointment(ctx, a.TenantID, id)
}

func (r *sqlRepository) UpdateAppointment(ctx context.Context, a billing.Appointment) (billing.Appointment, error) {
	a = a.Normalized()
	res, err := r.q().ExecContext(ctx,
		`UPDATE appointments SET account_id = ?, seller_id = ?, client_name = ?, phone = ?, unit = ?, address = ?,
		 device_count = ?, amount_cents = ?, currency = ?, scheduled_on = ?, time_window = ?, note = ?, updated_at = ?
		 WHERE id = ? AND tenant_id = ?`,
		nullInt64(a.AccountID), nullInt64(a.SellerID), a.ClientName, a.Phone, a.Unit, a.Address,
		a.DeviceCount, a.AmountCents, a.Currency, a.ScheduledOn, a.TimeWindow, a.Note, time.Now().UTC(), a.ID, a.TenantID)
	if err != nil {
		return billing.Appointment{}, fmt.Errorf("storage: update appointment: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return billing.Appointment{}, billing.ErrNotFound
	}
	return r.GetAppointment(ctx, a.TenantID, a.ID)
}

func (r *sqlRepository) GetAppointment(ctx context.Context, tenantID, appointmentID int64) (billing.Appointment, error) {
	a, err := scanAppointmentInto(r.q().QueryRowContext(ctx,
		`SELECT `+appointmentColumns+` FROM appointments WHERE tenant_id = ? AND id = ?`, tenantID, appointmentID))
	if errors.Is(err, sql.ErrNoRows) {
		return billing.Appointment{}, billing.ErrNotFound
	}
	if err != nil {
		return billing.Appointment{}, fmt.Errorf("storage: get appointment: %w", err)
	}
	return a, nil
}

func (r *sqlRepository) ListAppointments(ctx context.Context, tenantID int64, filter billing.AppointmentFilter) ([]billing.Appointment, error) {
	query := `SELECT ` + appointmentColumns + ` FROM appointments WHERE tenant_id = ?`
	args := []any{tenantID}

	if !filter.From.IsZero() {
		query += ` AND scheduled_on >= ?`
		args = append(args, filter.From)
	}
	if !filter.To.IsZero() {
		query += ` AND scheduled_on < ?`
		args = append(args, filter.To)
	}
	if filter.Status != "" {
		query += ` AND status = ?`
		args = append(args, string(filter.Status))
	}
	query += ` ORDER BY scheduled_on ASC, id ASC`

	rows, err := r.q().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("storage: list appointments: %w", err)
	}
	defer rows.Close()

	var appointments []billing.Appointment
	for rows.Next() {
		a, err := scanAppointmentInto(rows)
		if err != nil {
			return nil, fmt.Errorf("storage: scan appointment: %w", err)
		}
		appointments = append(appointments, a)
	}
	return appointments, rows.Err()
}

func (r *sqlRepository) SetAppointmentStatus(ctx context.Context, tenantID, appointmentID int64, status billing.AppointmentStatus, outcome string) (billing.Appointment, error) {
	// Reopening a visit clears the closing stamp, so a mistaken close does not
	// leave a date behind on a visit that is scheduled again.
	var closedAt any
	if status != billing.AppointmentScheduled {
		closedAt = time.Now().UTC()
	}
	res, err := r.q().ExecContext(ctx,
		`UPDATE appointments SET status = ?, outcome = ?, closed_at = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
		string(status), strings.TrimSpace(outcome), closedAt, time.Now().UTC(), appointmentID, tenantID)
	if err != nil {
		return billing.Appointment{}, fmt.Errorf("storage: set appointment status: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return billing.Appointment{}, billing.ErrNotFound
	}
	return r.GetAppointment(ctx, tenantID, appointmentID)
}

func (r *sqlRepository) DeleteAppointment(ctx context.Context, tenantID, appointmentID int64) error {
	res, err := r.q().ExecContext(ctx,
		`DELETE FROM appointments WHERE id = ? AND tenant_id = ?`, appointmentID, tenantID)
	if err != nil {
		return fmt.Errorf("storage: delete appointment: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return billing.ErrNotFound
	}
	return nil
}

// isUniqueViolation reports whether the driver rejected a write because it
// duplicates a unique key, which callers turn into billing.ErrConflict.
//
// modernc.org/sqlite reports extended codes (2067 for UNIQUE, 1299 for NOT
// NULL, 1555 for a primary key), so the SQLite arm matches 2067 alone. The
// bare 19 is deliberately not accepted: it is the whole SQLITE_CONSTRAINT
// class, and a caller that reads a NOT NULL failure as a duplicate would skip
// the row as work already done.
func isUniqueViolation(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.Code() == 2067
	}
	return false
}

const remissionColumns = `id, tenant_id, account_id, subscription_id, period_start, period_end, device_count, amount_cents, currency, status, note, payment_id, issued_at, paid_at, canceled_at, created_at, updated_at`

func scanRemissionInto(sc scanner) (billing.Remission, error) {
	var r billing.Remission
	var paymentID sql.NullInt64
	var paidAt, canceledAt sql.NullTime
	var status string
	if err := sc.Scan(
		&r.ID, &r.TenantID, &r.AccountID, &r.SubscriptionID,
		&r.PeriodStart, &r.PeriodEnd, &r.DeviceCount, &r.AmountCents,
		&r.Currency, &status, &r.Note, &paymentID,
		&r.IssuedAt, &paidAt, &canceledAt, &r.CreatedAt, &r.UpdatedAt,
	); err != nil {
		return billing.Remission{}, err
	}
	r.PaymentID = paymentID.Int64
	r.PaidAt = scanTime(paidAt)
	r.CanceledAt = scanTime(canceledAt)
	r.Status = billing.RemissionStatus(status)
	return r, nil
}

func (r *sqlRepository) CreateRemission(ctx context.Context, rem billing.Remission) (billing.Remission, error) {
	now := time.Now().UTC()
	if rem.IssuedAt.IsZero() {
		rem.IssuedAt = rem.PeriodStart
	}
	if rem.Status == "" {
		rem.Status = billing.RemissionPending
	}
	res, err := r.q().ExecContext(ctx,
		`INSERT INTO remissions (tenant_id, account_id, subscription_id, period_start, period_end, device_count, amount_cents, currency, status, note, payment_id, issued_at, paid_at, canceled_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rem.TenantID, rem.AccountID, rem.SubscriptionID, rem.PeriodStart, rem.PeriodEnd, rem.DeviceCount, rem.AmountCents, rem.Currency, string(rem.Status), rem.Note, nullInt64(rem.PaymentID), rem.IssuedAt, nullTime(rem.PaidAt), nullTime(rem.CanceledAt), now, now)
	if err != nil {
		if isUniqueViolation(err) {
			return billing.Remission{}, billing.ErrConflict
		}
		return billing.Remission{}, fmt.Errorf("storage: create remission: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return billing.Remission{}, fmt.Errorf("storage: create remission: %w", err)
	}
	return r.GetRemission(ctx, rem.TenantID, id)
}

func (r *sqlRepository) GetRemission(ctx context.Context, tenantID, remissionID int64) (billing.Remission, error) {
	rem, err := scanRemissionInto(r.q().QueryRowContext(ctx,
		`SELECT `+remissionColumns+` FROM remissions WHERE tenant_id = ? AND id = ?`, tenantID, remissionID))
	if errors.Is(err, sql.ErrNoRows) {
		return billing.Remission{}, billing.ErrNotFound
	}
	if err != nil {
		return billing.Remission{}, fmt.Errorf("storage: get remission: %w", err)
	}
	return rem, nil
}

func (r *sqlRepository) ListRemissions(ctx context.Context, tenantID int64, filter billing.RemissionFilter) ([]billing.TenantRemission, error) {
	query := `SELECT r.id, r.tenant_id, r.account_id, r.subscription_id, r.period_start, r.period_end, r.device_count, r.amount_cents, r.currency, r.status, r.note, r.payment_id, r.issued_at, r.paid_at, r.canceled_at, r.created_at, r.updated_at, a.name
		 FROM remissions r
		 JOIN accounts a ON a.id = r.account_id
		 WHERE r.tenant_id = ?`
	args := []any{tenantID}

	if !filter.From.IsZero() {
		query += ` AND r.period_start >= ?`
		args = append(args, filter.From.UTC())
	}
	if !filter.To.IsZero() {
		query += ` AND r.period_start <= ?`
		args = append(args, filter.To.UTC())
	}
	if filter.AccountID > 0 {
		query += ` AND r.account_id = ?`
		args = append(args, filter.AccountID)
	}
	if filter.Status != "" {
		query += ` AND r.status = ?`
		args = append(args, string(filter.Status))
	}
	query += ` ORDER BY r.period_start DESC, r.id DESC`

	rows, err := r.q().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("storage: list remissions: %w", err)
	}
	defer rows.Close()

	var remissions []billing.TenantRemission
	for rows.Next() {
		var tr billing.TenantRemission
		var paymentID sql.NullInt64
		var paidAt, canceledAt sql.NullTime
		var status string
		if err := rows.Scan(
			&tr.ID, &tr.TenantID, &tr.AccountID, &tr.SubscriptionID,
			&tr.PeriodStart, &tr.PeriodEnd, &tr.DeviceCount, &tr.AmountCents,
			&tr.Currency, &status, &tr.Note, &paymentID,
			&tr.IssuedAt, &paidAt, &canceledAt, &tr.CreatedAt, &tr.UpdatedAt,
			&tr.AccountName,
		); err != nil {
			return nil, fmt.Errorf("storage: scan remission: %w", err)
		}
		tr.PaymentID = paymentID.Int64
		tr.PaidAt = scanTime(paidAt)
		tr.CanceledAt = scanTime(canceledAt)
		tr.Status = billing.RemissionStatus(status)
		remissions = append(remissions, tr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: list remissions: %w", err)
	}
	return remissions, nil
}

func (r *sqlRepository) SettleRemission(ctx context.Context, tenantID, remissionID, paymentID int64, paidAt time.Time) (billing.Remission, error) {
	now := time.Now().UTC()
	res, err := r.q().ExecContext(ctx,
		`UPDATE remissions SET status = ?, payment_id = ?, paid_at = ?, updated_at = ?
		 WHERE id = ? AND tenant_id = ?`,
		string(billing.RemissionPaid), paymentID, paidAt, now, remissionID, tenantID)
	if err != nil {
		return billing.Remission{}, fmt.Errorf("storage: settle remission: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return billing.Remission{}, billing.ErrNotFound
	}
	return r.GetRemission(ctx, tenantID, remissionID)
}

func (r *sqlRepository) CancelRemission(ctx context.Context, tenantID, remissionID int64, canceledAt time.Time) error {
	now := time.Now().UTC()
	res, err := r.q().ExecContext(ctx,
		`UPDATE remissions SET status = ?, canceled_at = ?, updated_at = ?
		 WHERE id = ? AND tenant_id = ?`,
		string(billing.RemissionCanceled), canceledAt, now, remissionID, tenantID)
	if err != nil {
		return fmt.Errorf("storage: cancel remission: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return billing.ErrNotFound
	}
	return nil
}
