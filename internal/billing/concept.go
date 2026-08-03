package billing

import (
	"strings"
	"time"
)

// Concept represents a catalog item used to categorize charges recorded
// against subscriptions (e.g. installation fees, recurring monthly billing,
// or uninstallation fees).
type Concept struct {
	ID          int64
	TenantID    int64
	Name        string
	Slug        string
	AmountCents int64
	Currency    string
	Recurring   bool
	Active      bool
	Note        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Normalized returns a copy of Concept with Name and Note trimmed, Currency
// defaulted to MXN when unset, and Slug generated or sanitized into a lower-case,
// accent-free URL-friendly identifier.
func (c Concept) Normalized() Concept {
	c.Name = strings.TrimSpace(c.Name)
	c.Note = strings.TrimSpace(c.Note)
	c.Currency = strings.ToUpper(strings.TrimSpace(c.Currency))
	if c.Currency == "" {
		c.Currency = "MXN"
	}

	rawSlug := strings.TrimSpace(c.Slug)
	if rawSlug == "" {
		rawSlug = c.Name
	}
	c.Slug = Slugify(rawSlug)
	return c
}

// Slugify transforms raw strings into lower-case, ASCII-only identifiers by
// stripping common Spanish accents and turning punctuation or whitespace into
// single hyphens.
func Slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case 'á', 'à', 'ä', 'â', 'ã':
			b.WriteRune('a')
		case 'é', 'è', 'ë', 'ê':
			b.WriteRune('e')
		case 'í', 'ì', 'ï', 'î':
			b.WriteRune('i')
		case 'ó', 'ò', 'ö', 'ô', 'õ':
			b.WriteRune('o')
		case 'ú', 'ù', 'ü', 'û':
			b.WriteRune('u')
		case 'ñ':
			b.WriteRune('n')
		default:
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				b.WriteRune(r)
			} else {
				b.WriteRune('-')
			}
		}
	}
	res := b.String()
	for strings.Contains(res, "--") {
		res = strings.ReplaceAll(res, "--", "-")
	}
	return strings.Trim(res, "-")
}
