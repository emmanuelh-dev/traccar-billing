package billing

import "time"

func IsOverdue(sub Subscription, now time.Time) bool {
	if sub.Status == StatusCanceled {
		return false
	}
	return CutoffDate(sub).Before(now)
}

func CutoffDate(sub Subscription) time.Time {
	if sub.GraceDays <= 0 {
		return sub.NextDueAt
	}
	return sub.NextDueAt.AddDate(0, 0, sub.GraceDays)
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

func BillableDevices(sub Subscription, devices int) int {
	if devices < sub.MinDevices {
		devices = sub.MinDevices
	}
	if devices < 0 {
		return 0
	}
	return devices
}

func ChargeCents(sub Subscription, devices int) int64 {
	if !sub.PerDevice() {
		return sub.AmountCents
	}
	return int64(BillableDevices(sub, devices))*sub.UnitPriceCents + sub.FlatFeeCents
}
