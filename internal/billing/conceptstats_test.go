package billing

import (
	"testing"
	"time"
)

func TestConceptTotalsSplitsItemizedCharges(t *testing.T) {
	payments := []TenantPayment{
		{
			// A charge made of two lines paid for two different things. Reading
			// only the payment's own concept would credit all of it to one.
			Payment: Payment{
				AmountCents: 300000,
				ConceptID:   1,
				Items: []PaymentItem{
					{ConceptID: 1, Description: "Instalación", AmountCents: 100000},
					{ConceptID: 2, Description: "Equipo", AmountCents: 200000},
				},
			},
			ConceptName: "Instalación",
		},
		{
			Payment:     Payment{AmountCents: 50000, ConceptID: 2},
			ConceptName: "Equipo",
		},
	}

	totals := ConceptTotals(payments)
	if len(totals) != 2 {
		t.Fatalf("totals count = %d, want 2", len(totals))
	}
	if totals[0].ConceptID != 2 || totals[0].Cents != 250000 || totals[0].Count != 2 {
		t.Errorf("leader = %+v, want concept 2 with 250000 over 2 charges", totals[0])
	}
	if totals[1].ConceptID != 1 || totals[1].Cents != 100000 {
		t.Errorf("runner-up = %+v, want concept 1 with 100000", totals[1])
	}
}

func TestConceptTotalsSkipsVoided(t *testing.T) {
	payments := []TenantPayment{
		{Payment: Payment{AmountCents: 10000, ConceptID: 1}},
		{Payment: Payment{AmountCents: 99000, ConceptID: 1, VoidedAt: time.Now()}},
	}

	totals := ConceptTotals(payments)
	if len(totals) != 1 {
		t.Fatalf("totals count = %d, want 1", len(totals))
	}
	if totals[0].Cents != 10000 {
		t.Errorf("Cents = %d, want 10000: a voided payment is money that never arrived", totals[0].Cents)
	}
}

func TestConceptSeriesKeepsEmptyBuckets(t *testing.T) {
	loc := time.UTC
	from := time.Date(2026, time.August, 1, 0, 0, 0, 0, loc)
	to := from.AddDate(0, 0, 4)

	payments := []TenantPayment{
		{Payment: Payment{AmountCents: 1000, PaidAt: time.Date(2026, time.August, 1, 9, 0, 0, 0, loc)}},
		{Payment: Payment{AmountCents: 2500, PaidAt: time.Date(2026, time.August, 3, 23, 30, 0, 0, loc)}},
	}

	buckets := ConceptSeries(payments, from, to, ByDay, loc)
	if len(buckets) != 4 {
		t.Fatalf("bucket count = %d, want 4", len(buckets))
	}
	want := []int64{1000, 0, 2500, 0}
	for i, cents := range want {
		if buckets[i].Cents != cents {
			t.Errorf("bucket %d Cents = %d, want %d", i, buckets[i].Cents, cents)
		}
	}
	if !buckets[0].Start.Equal(from) {
		t.Errorf("first bucket starts at %s, want %s", buckets[0].Start, from)
	}
}

func TestConceptSeriesByMonth(t *testing.T) {
	loc := time.UTC
	from := time.Date(2026, time.January, 1, 0, 0, 0, 0, loc)
	to := from.AddDate(1, 0, 0)

	payments := []TenantPayment{
		{Payment: Payment{AmountCents: 500, PaidAt: time.Date(2026, time.January, 31, 0, 0, 0, 0, loc)}},
		{Payment: Payment{AmountCents: 700, PaidAt: time.Date(2026, time.January, 1, 0, 0, 0, 0, loc)}},
		{Payment: Payment{AmountCents: 300, PaidAt: time.Date(2026, time.December, 15, 0, 0, 0, 0, loc)}},
	}

	buckets := ConceptSeries(payments, from, to, ByMonth, loc)
	if len(buckets) != 12 {
		t.Fatalf("bucket count = %d, want 12", len(buckets))
	}
	if buckets[0].Cents != 1200 || buckets[0].Count != 2 {
		t.Errorf("January = %+v, want 1200 over 2 charges", buckets[0])
	}
	if buckets[11].Cents != 300 {
		t.Errorf("December Cents = %d, want 300", buckets[11].Cents)
	}
}

// A payment recorded late at night belongs to the day the operator sees on the
// receipt, not to the UTC day the timestamp happens to fall on.
func TestConceptSeriesBucketsInTenantTimezone(t *testing.T) {
	loc := time.FixedZone("CST", -6*3600)
	from := time.Date(2026, time.August, 1, 0, 0, 0, 0, loc)
	to := from.AddDate(0, 0, 3)

	// 2026-08-02 03:00 UTC is still 2026-08-01 21:00 locally.
	payments := []TenantPayment{
		{Payment: Payment{AmountCents: 4200, PaidAt: time.Date(2026, time.August, 2, 3, 0, 0, 0, time.UTC)}},
	}

	buckets := ConceptSeries(payments, from, to, ByDay, loc)
	if len(buckets) != 3 {
		t.Fatalf("bucket count = %d, want 3", len(buckets))
	}
	if buckets[0].Cents != 4200 {
		t.Errorf("first bucket Cents = %d, want 4200 (the payment belongs to the local day)", buckets[0].Cents)
	}
}
