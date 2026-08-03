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

func DayOfMonth(year int, month time.Month, day int, loc *time.Location) time.Time {
	lastDay := time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day()
	if day > lastDay {
		day = lastDay
	}
	if day < 1 {
		day = 1
	}
	return time.Date(year, month, day, 0, 0, 0, 0, loc)
}

func NextCalendarDue(after time.Time, dueDay int) time.Time {
	year, month, _ := after.Date()
	candidate := DayOfMonth(year, month, dueDay, after.Location())
	if candidate.After(after) {
		return candidate
	}
	return DayOfMonth(year, month+1, dueDay, after.Location())
}

func AdvanceDue(sub Subscription, paidAt time.Time) time.Time {
	if sub.Calendar() {
		return NextCalendarDue(sub.NextDueAt, sub.DueDay)
	}
	return NextDueDate(paidAt, sub.PeriodDays)
}

func ApplyPayment(sub Subscription, paidAt time.Time) Subscription {
	sub.LastPaidAt = paidAt
	sub.NextDueAt = AdvanceDue(sub, paidAt)
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
