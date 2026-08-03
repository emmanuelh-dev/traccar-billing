package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/yourusername/traccar-billing/internal/billing"
)

const (
	langCookieName   = "lang"
	viewCookieName   = "view"
	groupCookieName  = "group"
	mirrorCookieName = "mirror"
)

// resolveToggle backs a sticky on/off dashboard switch with a cookie, the
// same way resolveView backs the layout switch: ?name=1 or ?name=0 wins
// and is remembered, otherwise the cookie decides, defaulting to off.
func resolveToggle(w http.ResponseWriter, r *http.Request, name string) bool {
	switch r.URL.Query().Get(name) {
	case "1", "0":
		value := r.URL.Query().Get(name)
		http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", MaxAge: 365 * 24 * 3600, SameSite: http.SameSiteLaxMode})
		return value == "1"
	}
	if cookie, err := r.Cookie(name); err == nil {
		return cookie.Value == "1"
	}
	return false
}

var supportedViews = map[string]bool{"table": true, "cards": true}

// resolveView remembers whether the operator prefers the dense table or
// stacked cards, the same way resolveLang remembers the language: an
// explicit ?view= wins and is persisted, otherwise the cookie decides.
func resolveView(w http.ResponseWriter, r *http.Request) string {
	if view := r.URL.Query().Get("view"); supportedViews[view] {
		http.SetCookie(w, &http.Cookie{Name: viewCookieName, Value: view, Path: "/", MaxAge: 365 * 24 * 3600, SameSite: http.SameSiteLaxMode})
		return view
	}
	if cookie, err := r.Cookie(viewCookieName); err == nil && supportedViews[cookie.Value] {
		return cookie.Value
	}
	return "table"
}

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

	BillingModeLabel  string
	ModeRollingLabel  string
	ModeCalendarLabel string
	AnchorDayLabel    string
	DueDayLabel       string
	SessionExpiredTtl string
	SessionExpiredMsg string
	ReconnectButton   string

	NavSellers      string
	SellersPageTtl  string
	SellersEmpty    string
	NewSellerButton string
	NewSellerTitle  string
	EditSellerTitle string
	SellerLabel     string
	SellerNameLabel string
	PhoneLabel      string
	CommissionLabel string
	CommissionHint  string
	ActiveLabel     string
	ActiveYes       string
	ActiveNo        string
	NoSeller        string
	ColAccountsNum  string
	ColMonthly      string
	ColCommission   string
	AssignSellerTtl string
	UnassignOption  string

	DeleteButton      string
	DeletePaymentTtl  string
	DeleteWarning     string
	DeleteConfirmWord string
	DevicesWord       string
	EachWord          string
	ColBreakdown      string

	FilterPeriod   string
	PeriodCurrent  string
	PeriodPrevious string
	PeriodAll      string
	PeriodRange    string
	FromLabel      string
	ToLabel        string
	ApplyFilter    string
	AllAccounts    string
	VoidedCountFmt string
	NoPaymentsHere string
	ColUnitPrice   string
	ViewCards      string
	ViewTable      string

	NavSettings       string
	SettingsPageTtl   string
	SettingsIntro     string
	SettingsSaved     string
	DefaultsSection   string
	DisplaySection    string
	HideMirrorLabel   string
	HideMirrorHint    string
	MirrorBadge       string
	StatMirror        string
	ShowMirrorLink    string
	HideMirrorLink    string
	GroupBySeller     string
	GroupNone         string
	GroupTotalFmt     string
	DeleteAccountTtl  string
	DeleteAccountWarn string
	DeleteAccountBtn  string

	NavConcepts      string
	ConceptsPageTtl  string
	ConceptsEmpty    string
	NewConceptButton string
	NewConceptTitle  string
	EditConceptTitle string
	ConceptLabel     string
	ConceptNameLabel string
	SlugLabel        string
	SlugHint         string
	RecurringLabel   string
	RecurringYes     string
	RecurringNo      string
	NoConceptOption  string
	ColConcept       string
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

		BillingModeLabel:  "Tipo de ciclo",
		ModeRollingLabel:  "Por días corridos (desde el pago)",
		ModeCalendarLabel: "Por calendario (día fijo del mes)",
		AnchorDayLabel:    "Día de generación",
		DueDayLabel:       "Día de vencimiento",
		SessionExpiredTtl: "Sesión de Traccar vencida",
		SessionExpiredMsg: "No se están sincronizando las cuentas ni suspendiendo a los morosos. Vuelve a conectar tu servidor para reanudarlo.",
		ReconnectButton:   "Reconectar",

		NavSellers:      "Vendedores",
		SellersPageTtl:  "Vendedores",
		SellersEmpty:    "Todavía no hay vendedores. Da de alta uno para poder asignarle cuentas.",
		NewSellerButton: "Nuevo vendedor",
		NewSellerTitle:  "Nuevo vendedor",
		EditSellerTitle: "Editar vendedor",
		SellerLabel:     "Vendedor",
		SellerNameLabel: "Nombre",
		PhoneLabel:      "Teléfono",
		CommissionLabel: "Comisión (%)",
		CommissionHint:  "Porcentaje sobre el cobro mensual de sus cuentas. Se muestra como referencia; todavía no genera pagos de comisión.",
		ActiveLabel:     "Activo",
		ActiveYes:       "Sí",
		ActiveNo:        "No",
		NoSeller:        "sin vendedor",
		ColAccountsNum:  "Cuentas",
		ColMonthly:      "Cobro mensual",
		ColCommission:   "Comisión",
		AssignSellerTtl: "Asignar vendedor",
		UnassignOption:  "— sin vendedor —",

		NavConcepts:      "Conceptos",
		ConceptsPageTtl:  "Conceptos de cobro",
		ConceptsEmpty:    "Todavía no hay conceptos registrados. Da de alta uno para poder asociarlo a los pagos.",
		NewConceptButton: "Nuevo concepto",
		NewConceptTitle:  "Nuevo concepto",
		EditConceptTitle: "Editar concepto",
		ConceptLabel:     "Concepto",
		ConceptNameLabel: "Nombre",
		SlugLabel:        "Slug / Clave",
		SlugHint:         "Identificador único (minúsculas, sin acentos ni espacios). Se genera automáticamente si se deja en blanco.",
		RecurringLabel:   "Recurrente",
		RecurringYes:     "Sí",
		RecurringNo:      "No",
		NoConceptOption:  "— sin concepto —",
		ColConcept:       "Concepto",

		DeleteButton:      "Eliminar",
		DeletePaymentTtl:  "Eliminar pago",
		DeleteWarning:     "El pago se borra por completo y no se puede recuperar. Si solo quieres corregirlo, usa Anular y queda en el historial. Si este pago ya había movido la fecha de corte, ajústala en Configurar cobro.",
		DeleteConfirmWord: "Sí, eliminar",
		DevicesWord:       "dispositivos",
		EachWord:          "c/u",
		ColBreakdown:      "Concepto",

		FilterPeriod:   "Periodo",
		PeriodCurrent:  "Mes actual",
		PeriodPrevious: "Mes anterior",
		PeriodAll:      "Todo",
		PeriodRange:    "Rango",
		FromLabel:      "Desde",
		ToLabel:        "Hasta",
		ApplyFilter:    "Filtrar",
		AllAccounts:    "Todas las cuentas",
		VoidedCountFmt: "%d anulados, no suman al total",
		NoPaymentsHere: "No hay pagos en este periodo.",
		ColUnitPrice:   "Cobro por equipo",
		ViewCards:      "Ver como tarjetas",
		ViewTable:      "Ver como tabla",

		NavSettings:       "Ajustes",
		SettingsPageTtl:   "Ajustes de facturación",
		SettingsIntro:     "Estos valores se usan como punto de partida al configurar una cuenta nueva. Cambiarlos no afecta las suscripciones ya configuradas.",
		SettingsSaved:     "Ajustes guardados.",
		DefaultsSection:   "Valores por defecto",
		DisplaySection:    "Presentación",
		HideMirrorLabel:   "Ocultar cuentas espejo",
		HideMirrorHint:    "Traccar crea usuarios temporales al compartir un equipo (su correo lleva dos puntos y el ID del equipo). No son clientes y no se cobran.",
		MirrorBadge:       "espejo",
		StatMirror:        "Espejo",
		ShowMirrorLink:    "Mostrar espejo",
		HideMirrorLink:    "Ocultar espejo",
		GroupBySeller:     "Agrupar por vendedor",
		GroupNone:         "Quitar agrupación",
		GroupTotalFmt:     "%d cuentas · %d equipos · %s",
		DeleteAccountTtl:  "Eliminar cuenta",
		DeleteAccountWarn: "Se elimina la cuenta con su suscripción y todos sus pagos, y también se borra el usuario en Traccar. Esto no se puede deshacer.",
		DeleteAccountBtn:  "Eliminar cuenta",
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

		BillingModeLabel:  "Cycle type",
		ModeRollingLabel:  "Rolling days (from the payment)",
		ModeCalendarLabel: "Calendar (fixed day of month)",
		AnchorDayLabel:    "Generated on day",
		DueDayLabel:       "Due on day",
		SessionExpiredTtl: "Traccar session expired",
		SessionExpiredMsg: "Accounts are not syncing and overdue users are not being suspended. Reconnect your server to resume.",
		ReconnectButton:   "Reconnect",

		NavSellers:      "Sellers",
		SellersPageTtl:  "Sellers",
		SellersEmpty:    "No sellers yet. Add one so you can assign accounts to them.",
		NewSellerButton: "New seller",
		NewSellerTitle:  "New seller",
		EditSellerTitle: "Edit seller",
		SellerLabel:     "Seller",
		SellerNameLabel: "Name",
		PhoneLabel:      "Phone",
		CommissionLabel: "Commission (%)",
		CommissionHint:  "Percentage of the monthly billing of their accounts. Shown for reference; it does not generate commission payouts yet.",
		ActiveLabel:     "Active",
		ActiveYes:       "Yes",
		ActiveNo:        "No",
		NoSeller:        "no seller",
		ColAccountsNum:  "Accounts",
		ColMonthly:      "Monthly billing",
		ColCommission:   "Commission",
		AssignSellerTtl: "Assign seller",
		UnassignOption:  "— no seller —",

		NavConcepts:      "Concepts",
		ConceptsPageTtl:  "Billing concepts",
		ConceptsEmpty:    "No billing concepts registered yet. Create one to associate it with payments.",
		NewConceptButton: "New concept",
		NewConceptTitle:  "New concept",
		EditConceptTitle: "Edit concept",
		ConceptLabel:     "Concept",
		ConceptNameLabel: "Name",
		SlugLabel:        "Slug / Key",
		SlugHint:         "Unique identifier (lowercase, no accents or spaces). Auto-generated if left blank.",
		RecurringLabel:   "Recurring",
		RecurringYes:     "Yes",
		RecurringNo:      "No",
		NoConceptOption:  "— no concept —",
		ColConcept:       "Concept",

		DeleteButton:      "Delete",
		DeletePaymentTtl:  "Delete payment",
		DeleteWarning:     "The payment is removed for good and cannot be recovered. If you only want to correct it, use Void and it stays in the history. If this payment already moved the due date, adjust it in Set up billing.",
		DeleteConfirmWord: "Yes, delete",
		DevicesWord:       "devices",
		EachWord:          "each",
		ColBreakdown:      "Covers",

		FilterPeriod:   "Period",
		PeriodCurrent:  "This month",
		PeriodPrevious: "Last month",
		PeriodAll:      "All",
		PeriodRange:    "Range",
		FromLabel:      "From",
		ToLabel:        "To",
		ApplyFilter:    "Filter",
		AllAccounts:    "All accounts",
		VoidedCountFmt: "%d voided, not counted in the total",
		NoPaymentsHere: "No payments in this period.",
		ColUnitPrice:   "Price per unit",
		ViewCards:      "Card view",
		ViewTable:      "Table view",

		NavSettings:       "Settings",
		SettingsPageTtl:   "Billing settings",
		SettingsIntro:     "These values are the starting point when configuring a new account. Changing them leaves existing subscriptions untouched.",
		SettingsSaved:     "Settings saved.",
		DefaultsSection:   "Defaults",
		DisplaySection:    "Display",
		HideMirrorLabel:   "Hide mirror accounts",
		HideMirrorHint:    "Traccar creates temporary users when a device is shared (their email carries a colon and the device ID). They are not customers and are never billed.",
		MirrorBadge:       "mirror",
		StatMirror:        "Mirror",
		ShowMirrorLink:    "Show mirror",
		HideMirrorLink:    "Hide mirror",
		GroupBySeller:     "Group by seller",
		GroupNone:         "Ungroup",
		GroupTotalFmt:     "%d accounts · %d devices · %s",
		DeleteAccountTtl:  "Delete account",
		DeleteAccountWarn: "This deletes the account with its subscription and every payment, and removes the user in Traccar too. It cannot be undone.",
		DeleteAccountBtn:  "Delete account",
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

func (t uiStrings) voidedCountLabel(n int) string {
	return fmt.Sprintf(t.VoidedCountFmt, n)
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
