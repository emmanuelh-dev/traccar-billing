package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/yourusername/traccar-billing/internal/billing"
)

const langCookieName = "lang"

var supportedLangs = map[string]bool{"es": true, "en": true}

// resolveLang picks the UI language for this request: an explicit ?lang=
// query param wins and is persisted to a cookie, otherwise the existing
// cookie is used, otherwise the browser's Accept-Language, defaulting to
// Spanish.
func resolveLang(w http.ResponseWriter, r *http.Request) string {
	if lang := r.URL.Query().Get("lang"); supportedLangs[lang] {
		http.SetCookie(w, &http.Cookie{Name: langCookieName, Value: lang, Path: "/", MaxAge: 365 * 24 * 3600})
		return lang
	}
	if cookie, err := r.Cookie(langCookieName); err == nil && supportedLangs[cookie.Value] {
		return cookie.Value
	}
	if strings.HasPrefix(strings.ToLower(r.Header.Get("Accept-Language")), "en") {
		return "en"
	}
	return "es"
}

type uiStrings struct {
	Lang string

	LoginTitle         string
	LoginSubtitle      string
	BaseURLLabel       string
	BaseURLPlaceholder string
	BaseURLHelper      string
	EmailLabel         string
	PasswordLabel      string
	SignInButton       string
	LoginNote          string
	InvalidForm        string
	InvalidURL         string
	LoginFailed        string
	InternalError      string

	DashboardTitle string
	LogoutButton   string
	StatAccounts   string
	StatActive     string
	StatOverdue    string
	ColAccount     string
	ColEmail       string
	ColDevices     string
	ColAmount      string
	ColStatus      string
	ColNextDue     string
	ColDaysLeft    string
	EmptyState     string
	PayButton      string
	NoSubscription string
	ConfigureLabel string
	AmountLabel    string
	CurrencyLabel  string
	PeriodLabel    string
	DueDateLabel   string
	SaveButton     string
	DaysLeftFmt    string
	DaysOverdueFmt string

	PaymentsLabel    string
	PaymentsColDate  string
	PaymentsColAmnt  string
	PaymentsColNote  string
	PaymentsEmpty    string
	PaymentsColAccnt string
	PaymentsPageTtl  string
	NavDashboard     string
	NavPayments      string

	StatusActive    string
	StatusOverdue   string
	StatusSuspended string
	StatusCanceled  string

	ChargeTitle        string
	DevicesLabel       string
	UnitPriceLabel     string
	TotalLabel         string
	FixedAmountLabel   string
	FlatFeeLabel       string
	MinDevicesLabel    string
	GraceDaysLabel     string
	PricingHint        string
	NoSubscriptionWarn string
	PaidAtLabel        string
	MethodLabel        string
	MethodCash         string
	MethodTransfer     string
	MethodCard         string
	MethodOther        string
	ReferenceLabel     string
	NoteLabel          string
	CancelButton       string
	EditButton         string
	EditPaymentTitle   string
	VoidButton         string
	VoidedLabel        string
	VoidPaymentTitle   string
	VoidReasonLabel    string
	VoidWarning        string
	TotalCollected     string
}

var translations = map[string]uiStrings{
	"es": {
		Lang:               "es",
		LoginTitle:         "Iniciar sesión",
		LoginSubtitle:      "Conecta tu servidor Traccar para llevar el cobro de cuentas.",
		BaseURLLabel:       "URL del servidor Traccar",
		BaseURLPlaceholder: "https://tu-servidor.com",
		BaseURLHelper:      "No hace falta escribir /api, se agrega automáticamente.",
		EmailLabel:         "Correo",
		PasswordLabel:      "Contraseña",
		SignInButton:       "Iniciar sesión",
		LoginNote:          "Tu contraseña se usa una sola vez para autenticar contra tu servidor Traccar y nunca se guarda. Solo se conserva la sesión resultante.",
		InvalidForm:        "Formulario inválido.",
		InvalidURL:         "La URL del servidor Traccar no es válida.",
		LoginFailed:        "No se pudo iniciar sesión. Revisa la URL, el correo y la contraseña.",
		InternalError:      "Error interno.",

		DashboardTitle: "Dashboard",
		LogoutButton:   "Cerrar sesión",
		StatAccounts:   "Cuentas",
		StatActive:     "Al corriente",
		StatOverdue:    "Vencidas",
		ColAccount:     "Cuenta",
		ColEmail:       "Correo",
		ColDevices:     "Dispositivos",
		ColAmount:      "Cobro",
		ColStatus:      "Estado",
		ColNextDue:     "Próximo corte",
		ColDaysLeft:    "Faltan",
		EmptyState:     "Aún no hay cuentas sincronizadas. Se sincronizan automáticamente cada pocos minutos.",
		PayButton:      "Registrar pago",
		NoSubscription: "sin configurar",
		ConfigureLabel: "Configurar cobro",
		AmountLabel:    "Monto",
		CurrencyLabel:  "Moneda",
		PeriodLabel:    "Días por período",
		DueDateLabel:   "Próxima fecha de corte",
		SaveButton:     "Guardar",
		DaysLeftFmt:    "%d días",
		DaysOverdueFmt: "vencida hace %d días",

		PaymentsLabel:    "Historial de pagos",
		PaymentsColDate:  "Fecha",
		PaymentsColAmnt:  "Monto",
		PaymentsColNote:  "Nota",
		PaymentsEmpty:    "Sin pagos registrados.",
		PaymentsColAccnt: "Cuenta",
		PaymentsPageTtl:  "Pagos",
		NavDashboard:     "Dashboard",
		NavPayments:      "Pagos",

		StatusActive:    "al corriente",
		StatusOverdue:   "vencida",
		StatusSuspended: "suspendida",
		StatusCanceled:  "cancelada",

		ChargeTitle:        "Registrar pago",
		DevicesLabel:       "Dispositivos a cobrar",
		UnitPriceLabel:     "Precio por dispositivo",
		TotalLabel:         "Total",
		FixedAmountLabel:   "Monto fijo",
		FlatFeeLabel:       "Cargo base",
		MinDevicesLabel:    "Mínimo facturable",
		GraceDaysLabel:     "Días de gracia",
		PricingHint:        "Usa el precio por dispositivo para cobrar según cuántos GPS tenga la cuenta, o el monto fijo para cobrar siempre lo mismo.",
		NoSubscriptionWarn: "Esta cuenta todavía no tiene cobro configurado.",
		PaidAtLabel:        "Fecha de pago",
		MethodLabel:        "Método",
		MethodCash:         "Efectivo",
		MethodTransfer:     "Transferencia",
		MethodCard:         "Tarjeta",
		MethodOther:        "Otro",
		ReferenceLabel:     "Referencia",
		NoteLabel:          "Nota",
		CancelButton:       "Cancelar",
		EditButton:         "Editar",
		EditPaymentTitle:   "Editar pago",
		VoidButton:         "Anular",
		VoidedLabel:        "anulado",
		VoidPaymentTitle:   "Anular pago",
		VoidReasonLabel:    "Motivo",
		VoidWarning:        "El pago se conserva en el historial marcado como anulado. Si ya había movido la fecha de corte, ajústala en Configurar cobro.",
		TotalCollected:     "Total cobrado",
	},
	"en": {
		Lang:               "en",
		LoginTitle:         "Sign in",
		LoginSubtitle:      "Connect your Traccar server to manage account billing.",
		BaseURLLabel:       "Traccar server URL",
		BaseURLPlaceholder: "https://your-server.com",
		BaseURLHelper:      "No need to type /api, it's added automatically.",
		EmailLabel:         "Email",
		PasswordLabel:      "Password",
		SignInButton:       "Sign in",
		LoginNote:          "Your password is used once to authenticate against your Traccar server and is never stored. Only the resulting session is kept.",
		InvalidForm:        "Invalid form.",
		InvalidURL:         "The Traccar server URL is not valid.",
		LoginFailed:        "Could not sign in. Check the URL, email, and password.",
		InternalError:      "Internal error.",

		DashboardTitle: "Dashboard",
		LogoutButton:   "Sign out",
		StatAccounts:   "Accounts",
		StatActive:     "Current",
		StatOverdue:    "Overdue",
		ColAccount:     "Account",
		ColEmail:       "Email",
		ColDevices:     "Devices",
		ColAmount:      "Billing",
		ColStatus:      "Status",
		ColNextDue:     "Next due date",
		ColDaysLeft:    "Left",
		EmptyState:     "No accounts synced yet. They sync automatically every few minutes.",
		PayButton:      "Record payment",
		NoSubscription: "not set up",
		ConfigureLabel: "Set up billing",
		AmountLabel:    "Amount",
		CurrencyLabel:  "Currency",
		PeriodLabel:    "Billing period (days)",
		DueDateLabel:   "Next due date",
		SaveButton:     "Save",
		DaysLeftFmt:    "%d days",
		DaysOverdueFmt: "%d days overdue",

		PaymentsLabel:    "Payment history",
		PaymentsColDate:  "Date",
		PaymentsColAmnt:  "Amount",
		PaymentsColNote:  "Note",
		PaymentsEmpty:    "No payments recorded yet.",
		PaymentsColAccnt: "Account",
		PaymentsPageTtl:  "Payments",
		NavDashboard:     "Dashboard",
		NavPayments:      "Payments",

		StatusActive:    "current",
		StatusOverdue:   "overdue",
		StatusSuspended: "suspended",
		StatusCanceled:  "canceled",

		ChargeTitle:        "Record payment",
		DevicesLabel:       "Devices to charge",
		UnitPriceLabel:     "Price per device",
		TotalLabel:         "Total",
		FixedAmountLabel:   "Flat amount",
		FlatFeeLabel:       "Base fee",
		MinDevicesLabel:    "Minimum billable",
		GraceDaysLabel:     "Grace days",
		PricingHint:        "Use price per device to bill by how many GPS units the account has, or the flat amount to always charge the same.",
		NoSubscriptionWarn: "This account has no billing set up yet.",
		PaidAtLabel:        "Payment date",
		MethodLabel:        "Method",
		MethodCash:         "Cash",
		MethodTransfer:     "Transfer",
		MethodCard:         "Card",
		MethodOther:        "Other",
		ReferenceLabel:     "Reference",
		NoteLabel:          "Note",
		CancelButton:       "Cancel",
		EditButton:         "Edit",
		EditPaymentTitle:   "Edit payment",
		VoidButton:         "Void",
		VoidedLabel:        "voided",
		VoidPaymentTitle:   "Void payment",
		VoidReasonLabel:    "Reason",
		VoidWarning:        "The payment stays in the history marked as voided. If it already moved the due date, adjust it in Set up billing.",
		TotalCollected:     "Total collected",
	},
}

func stringsFor(lang string) uiStrings {
	return translations[lang]
}

func (t uiStrings) daysLeftLabel(days int) string {
	if days < 0 {
		return fmt.Sprintf(t.DaysOverdueFmt, -days)
	}
	return fmt.Sprintf(t.DaysLeftFmt, days)
}

func (t uiStrings) statusLabel(status billing.SubscriptionStatus) string {
	switch status {
	case billing.StatusActive:
		return t.StatusActive
	case billing.StatusOverdue:
		return t.StatusOverdue
	case billing.StatusSuspended:
		return t.StatusSuspended
	case billing.StatusCanceled:
		return t.StatusCanceled
	default:
		return string(status)
	}
}
