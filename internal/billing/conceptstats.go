package billing

import (
	"sort"
	"time"
)

// Granularity is the width of one column in the concept chart.
type Granularity string

const (
	ByDay   Granularity = "day"
	ByMonth Granularity = "month"
)

// ConceptTotal is what one concept brought in over a period.
type ConceptTotal struct {
	ConceptID int64
	Name      string
	Cents     int64
	Count     int
}

// ConceptBucket is one column of the chart: a slice of time and the money
// that landed in it.
type ConceptBucket struct {
	Start time.Time
	Cents int64
	Count int
}

// conceptShares splits a payment into the concepts it actually paid for.
//
// A charge recorded as several lines paid for several things at once, and
// collapsing it onto the payment's single concept id would credit, say, an
// installation with the equipment that was sold alongside it. The payment row
// only keeps one concept because that is all the column can hold; the lines
// are where the truth is, so they win whenever they exist.
func conceptShares(p TenantPayment) []ConceptTotal {
	if len(p.Items) == 0 {
		return []ConceptTotal{{ConceptID: p.ConceptID, Name: p.ConceptName, Cents: p.AmountCents, Count: 1}}
	}
	shares := make([]ConceptTotal, 0, len(p.Items))
	for _, item := range p.Items {
		shares = append(shares, ConceptTotal{
			ConceptID: item.ConceptID,
			Name:      item.Description,
			Cents:     item.Total(),
			Count:     1,
		})
	}
	return shares
}

// ConceptTotals ranks concepts by how much money they brought in, biggest
// first. Voided payments are money that never arrived, so they are left out
// the same way the payments page leaves them out of its total.
func ConceptTotals(payments []TenantPayment) []ConceptTotal {
	byConcept := make(map[int64]*ConceptTotal)
	var order []int64
	for _, p := range payments {
		if p.Voided() {
			continue
		}
		for _, share := range conceptShares(p) {
			total, seen := byConcept[share.ConceptID]
			if !seen {
				copied := share
				byConcept[share.ConceptID] = &copied
				order = append(order, share.ConceptID)
				continue
			}
			total.Cents += share.Cents
			total.Count += share.Count
			// A concept renamed after some payments were recorded leaves older
			// rows carrying the old name, so any name is better than none.
			if total.Name == "" {
				total.Name = share.Name
			}
		}
	}

	totals := make([]ConceptTotal, 0, len(order))
	for _, id := range order {
		totals = append(totals, *byConcept[id])
	}
	sort.Slice(totals, func(i, j int) bool {
		if totals[i].Cents != totals[j].Cents {
			return totals[i].Cents > totals[j].Cents
		}
		return totals[i].ConceptID < totals[j].ConceptID
	})
	return totals
}

// ConceptSeries lays the same money out over time, one bucket per day or per
// month. Empty buckets are kept: a week with no income is something the
// operator needs to see, and dropping it would quietly stretch the chart.
//
// The range is half-open [from, to). A zero from or to falls back to the span
// the payments themselves cover.
func ConceptSeries(payments []TenantPayment, from, to time.Time, g Granularity, loc *time.Location) []ConceptBucket {
	live := make([]TenantPayment, 0, len(payments))
	for _, p := range payments {
		if !p.Voided() {
			live = append(live, p)
		}
	}
	if len(live) == 0 {
		return nil
	}

	if from.IsZero() || to.IsZero() {
		first, last := live[0].PaidAt, live[0].PaidAt
		for _, p := range live {
			if p.PaidAt.Before(first) {
				first = p.PaidAt
			}
			if p.PaidAt.After(last) {
				last = p.PaidAt
			}
		}
		if from.IsZero() {
			from = first
		}
		if to.IsZero() {
			to = advanceBucket(truncateTo(last, g, loc), g)
		}
	}

	start := truncateTo(from, g, loc)
	if !start.Before(to) {
		return nil
	}

	index := make(map[time.Time]int)
	var buckets []ConceptBucket
	for at := start; at.Before(to); at = advanceBucket(at, g) {
		index[at] = len(buckets)
		buckets = append(buckets, ConceptBucket{Start: at})
	}

	for _, p := range live {
		i, ok := index[truncateTo(p.PaidAt, g, loc)]
		if !ok {
			continue
		}
		buckets[i].Cents += p.AmountCents
		buckets[i].Count++
	}
	return buckets
}

func truncateTo(at time.Time, g Granularity, loc *time.Location) time.Time {
	local := at.In(loc)
	if g == ByMonth {
		return time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, loc)
	}
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}

func advanceBucket(at time.Time, g Granularity) time.Time {
	if g == ByMonth {
		return at.AddDate(0, 1, 0)
	}
	return at.AddDate(0, 0, 1)
}
