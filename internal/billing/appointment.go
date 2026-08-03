package billing

import (
	"strings"
	"time"
)

// AppointmentStatus is where a visit stands. A visit is only ever opened,
// closed once it happened, or canceled if it did not.
type AppointmentStatus string

const (
	AppointmentScheduled AppointmentStatus = "scheduled"
	AppointmentDone      AppointmentStatus = "done"
	AppointmentCanceled  AppointmentStatus = "canceled"
)

// Appointment is a scheduled visit: an installation, a check-up or a pickup.
// It exists before the customer does, so AccountID and SellerID may be zero.
type Appointment struct {
	ID          int64
	TenantID    int64
	AccountID   int64
	SellerID    int64
	ClientName  string
	Phone       string
	Unit        string
	Address     string
	DeviceCount int
	AmountCents int64
	Currency    string
	ScheduledOn time.Time
	TimeWindow  string
	Status      AppointmentStatus
	Note        string
	Outcome     string
	ClosedAt    time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (a Appointment) Open() bool {
	return a.Status == AppointmentScheduled
}

func (a Appointment) Normalized() Appointment {
	a.ClientName = strings.TrimSpace(a.ClientName)
	a.Phone = strings.TrimSpace(a.Phone)
	a.Unit = strings.TrimSpace(a.Unit)
	a.Address = strings.TrimSpace(a.Address)
	a.TimeWindow = strings.TrimSpace(a.TimeWindow)
	a.Note = strings.TrimSpace(a.Note)
	a.Outcome = strings.TrimSpace(a.Outcome)
	a.Currency = strings.ToUpper(strings.TrimSpace(a.Currency))
	if a.Currency == "" {
		a.Currency = "MXN"
	}
	if a.Status == "" {
		a.Status = AppointmentScheduled
	}
	if a.DeviceCount < 1 {
		a.DeviceCount = 1
	}
	return a
}

// PhoneNumbers splits the contact field, which operators fill in as a
// comma-separated list because a job occasionally has a second contact.
func (a Appointment) PhoneNumbers() []string {
	var numbers []string
	for _, part := range strings.Split(a.Phone, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			numbers = append(numbers, trimmed)
		}
	}
	return numbers
}

// AppointmentFilter narrows the agenda list. A zero value means every visit.
type AppointmentFilter struct {
	From   time.Time
	To     time.Time
	Status AppointmentStatus
}
