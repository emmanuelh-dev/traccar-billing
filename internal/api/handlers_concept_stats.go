package api

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/yourusername/traccar-billing/internal/billing"
)

// conceptChartBar is one column of the income-over-time chart. Its height is
// worked out here rather than in the template, which is kept free of
// arithmetic like every other page in this app.
type conceptChartBar struct {
	Label string
	Title string
	Style template.CSS
	Zero  bool
}

// conceptShareRow is one line of the per-concept ranking.
type conceptShareRow struct {
	Name          string
	AmountDisplay string
	Count         int
	SharePct      string
	Style         template.CSS
	Tone          int
}

type conceptYearOption struct {
	Year     int
	Selected bool
}

type conceptMonthOption struct {
	Month    int
	Name     string
	Selected bool
}

// shareTones is how many bar colours the palette has. The ranking cycles
// through them so two adjacent concepts never share one.
const shareTones = 6

// conceptChartMinBar keeps a real but tiny amount from rendering as nothing at
// all, which would read as a day with no income.
const conceptChartMinBar = 2.0

// resolveConceptPeriod turns ?year= and ?month= into a half-open [from, to)
// range in the tenant's timezone. Month zero means the whole year, which is
// the view that answers "how did this concept do over the year".
func (s *Server) resolveConceptPeriod(r *http.Request) (int, int, billing.PaymentFilter) {
	now := s.now()

	year := now.Year()
	if v, err := strconv.Atoi(r.URL.Query().Get("year")); err == nil && v >= 2000 && v <= now.Year()+1 {
		year = v
	}

	month := int(now.Month())
	if raw := r.URL.Query().Get("month"); raw != "" {
		if raw == "all" {
			month = 0
		} else if v, err := strconv.Atoi(raw); err == nil && v >= 1 && v <= 12 {
			month = v
		}
	}
	// A year other than the current one defaults to the whole year: asking for
	// 2025 and landing on whichever month today happens to be is never useful.
	if r.URL.Query().Get("month") == "" && year != now.Year() {
		month = 0
	}

	if month == 0 {
		from := time.Date(year, time.January, 1, 0, 0, 0, 0, s.loc)
		return year, month, billing.PaymentFilter{From: from, To: from.AddDate(1, 0, 0)}
	}
	from := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, s.loc)
	return year, month, billing.PaymentFilter{From: from, To: from.AddDate(0, 1, 0)}
}

// resolveGranularity picks the bucket width. A single month is only readable
// day by day and a whole year only month by month, so the default follows the
// period and the operator can still override it.
func resolveGranularity(r *http.Request, month int) billing.Granularity {
	switch r.URL.Query().Get("by") {
	case "day":
		return billing.ByDay
	case "month":
		return billing.ByMonth
	}
	if month == 0 {
		return billing.ByMonth
	}
	return billing.ByDay
}

func (s *Server) conceptYearOptions(selected int) []conceptYearOption {
	current := s.now().Year()
	opts := make([]conceptYearOption, 0, 5)
	for y := current; y > current-5; y-- {
		opts = append(opts, conceptYearOption{Year: y, Selected: y == selected})
	}
	return opts
}

func conceptMonthOptions(t uiStrings, selected int) []conceptMonthOption {
	opts := make([]conceptMonthOption, 0, 12)
	for m := 1; m <= 12; m++ {
		opts = append(opts, conceptMonthOption{Month: m, Name: t.MonthNames[m-1], Selected: m == selected})
	}
	return opts
}

// conceptChartBars turns the buckets into columns scaled against the biggest
// one. Axis labels are thinned out on daily charts because thirty-one of them
// under a narrow chart is an unreadable smear.
func conceptChartBars(buckets []billing.ConceptBucket, g billing.Granularity, t uiStrings, currency string) []conceptChartBar {
	var peak int64
	for _, b := range buckets {
		if b.Cents > peak {
			peak = b.Cents
		}
	}

	bars := make([]conceptChartBar, 0, len(buckets))
	for i, b := range buckets {
		bar := conceptChartBar{Zero: b.Cents == 0}

		height := 0.0
		if peak > 0 && b.Cents > 0 {
			height = float64(b.Cents) / float64(peak) * 100
			if height < conceptChartMinBar {
				height = conceptChartMinBar
			}
		}
		bar.Style = template.CSS(fmt.Sprintf("height:%.2f%%", height))

		if g == billing.ByMonth {
			bar.Label = t.MonthNames[int(b.Start.Month())-1][:3]
			bar.Title = fmt.Sprintf("%s %d — %s", t.MonthNames[int(b.Start.Month())-1], b.Start.Year(), formatAmount(b.Cents, currency))
			bars = append(bars, bar)
			continue
		}

		day := b.Start.Day()
		if day == 1 || day%5 == 0 || i == len(buckets)-1 {
			bar.Label = strconv.Itoa(day)
		}
		bar.Title = fmt.Sprintf("%s — %s", b.Start.Format(dueDateFormat), formatAmount(b.Cents, currency))
		bars = append(bars, bar)
	}
	return bars
}

// conceptShareRows ranks the concepts and sizes each bar against the leader,
// so the list reads as a chart rather than as a column of numbers.
func conceptShareRows(totals []billing.ConceptTotal, t uiStrings, currency string) []conceptShareRow {
	var grand int64
	for _, total := range totals {
		grand += total.Cents
	}
	if grand == 0 {
		return nil
	}
	leader := totals[0].Cents

	rows := make([]conceptShareRow, 0, len(totals))
	for i, total := range totals {
		name := total.Name
		if name == "" {
			// A line with no concept is the plain monthly fee, which is what
			// the charge modal calls it too.
			name = t.MonthlyLineOption
		}
		width := 0.0
		if leader > 0 {
			width = float64(total.Cents) / float64(leader) * 100
		}
		rows = append(rows, conceptShareRow{
			Name:          name,
			AmountDisplay: formatAmount(total.Cents, currency),
			Count:         total.Count,
			SharePct:      fmt.Sprintf("%.1f%%", float64(total.Cents)/float64(grand)*100),
			Style:         template.CSS(fmt.Sprintf("width:%.2f%%", width)),
			Tone:          i%shareTones + 1,
		})
	}
	return rows
}

// conceptStatsCurrency reports the currency the chart is drawn in. Mixing
// currencies in one total would be wrong, so the most recent payment decides
// and the rest is a rounding problem the operator can see in the table.
func conceptStatsCurrency(payments []billing.TenantPayment) string {
	for _, p := range payments {
		if !p.Voided() && p.Currency != "" {
			return p.Currency
		}
	}
	return defaultCurrency
}

// livePaymentCount counts the charges behind the chart. Voided ones are money
// that never arrived, so counting them would put a number on the page that
// does not match the total right next to it.
func livePaymentCount(payments []billing.TenantPayment) int {
	n := 0
	for _, p := range payments {
		if !p.Voided() {
			n++
		}
	}
	return n
}

// conceptStatsQuery rebuilds the page's own querystring so the filter form and
// the granularity chips can link back to the same view.
func conceptStatsQuery(year, month int, by billing.Granularity) string {
	m := "all"
	if month > 0 {
		m = strconv.Itoa(month)
	}
	return fmt.Sprintf("year=%d&month=%s&by=%s", year, m, by)
}
