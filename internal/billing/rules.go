package billing

import "time"

func IsOverdue(sub Subscription, now time.Time) bool {
	if sub.Status == StatusCanceled {
		return false
	}
	return sub.NextDueAt.Before(now)
}

func NextDueDate(from time.Time, periodDays int) time.Time {
	return from.AddDate(0, 0, periodDays)
}

func ApplyPayment(sub Subscription, paidAt time.Time) Subscription {
	sub.LastPaidAt = paidAt
	sub.NextDueAt = NextDueDate(paidAt, sub.PeriodDays)
	sub.Status = StatusActive
	return sub
}
