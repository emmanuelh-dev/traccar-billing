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
	sortCookieName   = "sort"

	appointmentStatusCookieName = "agenda"

	expenseSortCookieName = "esort"
	paymentSortCookieName = "psort"
)

// resolveChoice backs a sticky multi-value preference with a cookie: an
// explicit query param wins and is remembered, otherwise the cookie decides,
// otherwise the given default. Unknown values fall through so a stale link or
// a tampered cookie cannot leave the page ordered by nothing.
func resolveChoice(w http.ResponseWriter, r *http.Request, name string, allowed map[string]bool, fallback string) string {
	if value := r.URL.Query().Get(name); allowed[value] {
		http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", MaxAge: 365 * 24 * 3600, SameSite: http.SameSiteLaxMode})
		return value
	}
	if cookie, err := r.Cookie(name); err == nil && allowed[cookie.Value] {
		return cookie.Value
	}
	return fallback
}

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

	DashboardTitle            string
	NavDevices                string
	NavSIMs                   string
	NavSIMHistory             string
	DevicesPageTtl            string
	DevicesLoading            string
	DevicesLoadingUsage       string
	DeviceSearchLabel         string
	DeviceSearchHint          string
	DevicesSortLabel          string
	DevicesSortProvider       string
	DevicesSortName           string
	DeviceNameCol             string
	DataAllowedCol            string
	DataUsedCol               string
	DataRemainingCol          string
	DataRemainingCalculated   string
	ServicePackCol            string
	TimeRemainingCol          string
	UsagePeriodCol            string
	UsageUpdatedLabel         string
	UsageUnavailable          string
	UsageReportUnavailable    string
	DevicesEmpty              string
	DevicesFetchError         string
	ConnectivityNotConfigured string
	DeviceProtocolFilterWarn  string
	SMSAction                 string
	SMSDialogTitle            string
	SMSTemplateLabel          string
	SMSCustomOption           string
	SMSInfoOption             string
	SMSGPSOption              string
	SMSMessageLabel           string
	SMSHistoryTitle           string
	SMSHistoryEmpty           string
	SMSHistoryLoading         string
	SMSSendButton             string
	SMSSentNotice             string
	SMSSendError              string
	SIMHistoryPageTtl         string
	SIMHistoryPageSub         string
	SIMHistorySearch          string
	SIMHistorySearchHint      string
	SIMHistoryDateCol         string
	SIMHistoryDirCol          string
	SIMHistoryReceived        string
	SIMHistorySent            string
	SIMHistoryMessageCol      string
	SMSResendBtn              string
	SMSHistoryUnavailable     string
	ConnectivitySection       string
	ConnectivityIntro         string
	ConnectivityActive        string
	ConnectivityMissing       string
	ConnectivityProviderLabel string
	ConnectivityTokenLabel    string
	ConnectivityTokenHint     string
	ConnectivitySave          string
	ConnectivityRemove        string
	SIMsPageTtl               string
	SIMsPageSub               string
	SIMsLoading               string
	SIMsEmpty                 string
	SIMLabelCol               string
	SIMActivatedCol           string
	SIMInventoryUnavailable   string
	SIMActionsCol             string
	SIMActivateBtn            string
	SIMSuspendBtn             string
	SIMActivateConfirm        string
	SIMSuspendConfirm         string
	SIMStatusUpdated          string
	SIMStatusFailed           string
	SIMStatusUnavailable      string
	SIMStatusInvalid          string
	SIMRefreshBtn             string
	SIMRefreshing             string
	SIMRefreshFailed          string
	GoToSettings              string
	LogoutButton              string
	SignedInAs                string
	StatAccounts              string
	StatActive                string
	StatOverdue               string
	ColAccount                string
	ColEmail                  string
	ColDevices                string
	ColAmount                 string
	ColStatus                 string
	ColNextDue                string
	ColDaysLeft               string
	EmptyState                string
	PayButton                 string
	NoSubscription            string
	ConfigureLabel            string
	ResetPeriodLabel          string
	ResetPeriodConfirm        string
	AmountLabel               string
	CurrencyLabel             string
	PeriodLabel               string
	DueDateLabel              string
	SaveButton                string
	DaysLeftFmt               string
	DaysOverdueFmt            string

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
	MenuButton         string
	CloseButton        string
	SortLabel          string
	TotalsLabel        string
	QuantityLabel      string
	LineAmountLabel    string
	AddLineButton      string
	RemoveLineButton   string
	MonthlyLineOption  string
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
	ToggleEmails   string

	NavSettings     string
	SettingsPageTtl string
	SettingsIntro   string
	SettingsSaved   string
	DefaultsSection string
	DisplaySection  string
	HideMirrorLabel string
	HideMirrorHint  string

	RemissionsPageTtl    string
	NavRemissions        string
	RemissionPendingLbl  string
	RemissionPaidLabel   string
	RemissionCanceledLbl string
	RemissionAccountCol  string
	RemissionPeriodCol   string
	RemissionDevicesCol  string
	RemissionAmountCol   string
	RemissionStatusCol   string
	RemissionMarkPaidBtn string
	RemissionCancelBtn   string
	RemissionEmpty       string
	RemissionPendingSum  string
	RemissionUnbilledFmt string
	RemissionIntro       string

	ConnectionSection string
	TokenIntro        string
	TokenActiveLabel  string
	TokenMissingLabel string
	TokenMissingHint  string
	TokenPasteLabel   string
	TokenPasteHint    string
	TokenGenerateBtn  string
	TokenRemoveBtn    string
	TokenSaveBtn      string
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

	NavConcepts     string
	ConceptsPageTtl string
	ConceptsEmpty   string

	MonthNames       [12]string
	IncomeByConcept  string
	YearLabel        string
	MonthLabel       string
	AllMonthsOption  string
	GroupByDay       string
	GroupByMonth     string
	ChargesCountLbl  string
	TopConceptLabel  string
	NoIncomeInPeriod string
	ShareLabel       string
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
	OneOffChargeFmt  string
	OneOffBadge      string

	NavExpenses     string
	ExpensesPageTtl string

	CategoryHint        string
	DefaultCategories   []string
	NavAppointments     string
	AppointmentsPageTtl string
	AppointmentsEmpty   string
	NewAppointmentBtn   string
	NewAppointmentTtl   string
	EditAppointmentTtl  string
	ClientLabel         string
	ContactLabel        string
	ContactHint         string
	UnitLabel           string
	AddressLabel        string
	TimeWindowLabel     string
	TimeWindowHint      string
	AltasLabel          string
	CostLabel           string
	OutcomeLabel        string
	VisitScheduled      string
	VisitDone           string
	VisitCanceled       string
	CloseVisitBtn       string
	CloseVisitTtl       string
	CancelVisitBtn      string
	CancelVisitTtl      string
	ReopenVisitBtn      string
	DeleteVisitTtl      string
	WhatsAppButton      string
	WhatsAppWebButton   string
	WhatsAppMsgFmt      string
	WhatsAppUnitFmt     string
	WhatsAppAddrFmt     string
	LateBadge           string
	PendingVisits       string
	DoneVisits          string
	PendingAmount       string
	FilterOpen          string
	FilterDone          string
	FilterCanceled      string
	ExpensesEmpty       string
	NoExpensesHere      string
	NewExpenseButton    string
	NewExpenseTitle     string
	EditExpenseTitle    string
	DeleteExpenseTtl    string
	CategoryLabel       string
	TotalWithdrawn      string
	NetTotal            string
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

		DashboardTitle:            "Dashboard",
		NavDevices:                "Dispositivos",
		NavSIMs:                   "SIMs",
		NavSIMHistory:             "SMS",
		DevicesPageTtl:            "Consumo de dispositivos",
		DevicesLoading:            "Cargando dispositivos y consumo en segundo plano…",
		DevicesLoadingUsage:       "Completando datos de SIM y consumo…",
		DeviceSearchLabel:         "Buscar",
		DeviceSearchHint:          "Nombre, IMEI o ICCID",
		DevicesSortLabel:          "Ordenar",
		DevicesSortProvider:       "Con SIM primero",
		DevicesSortName:           "Nombre",
		DeviceNameCol:             "Dispositivo",
		DataAllowedCol:            "Datos incluidos",
		DataUsedCol:               "Datos gastados",
		DataRemainingCol:          "Datos restantes",
		DataRemainingCalculated:   "Saldo calculado con el límite del plan y el consumo del periodo completo de esta SIM.",
		ServicePackCol:            "Plan",
		TimeRemainingCol:          "Tiempo restante",
		UsagePeriodCol:            "Periodo consultado",
		UsageUpdatedLabel:         "Consultado",
		UsageUnavailable:          "Sin información de 1GLOBAL para este IMEI.",
		UsageReportUnavailable:    "Se identificaron las SIM, pero 1GLOBAL no pudo generar el reporte de consumo.",
		DevicesEmpty:              "Traccar no devolvió dispositivos.",
		DevicesFetchError:         "No se pudieron consultar los dispositivos de Traccar.",
		ConnectivityNotConfigured: "Configura tu proveedor y API key en Ajustes para consultar SIMs y consumo.",
		DeviceProtocolFilterWarn:  "No se pudo consultar el protocolo; solo se ocultaron los dispositivos recovery.",
		SMSAction:                 "Mensajes",
		SMSDialogTitle:            "Enviar mensaje al dispositivo",
		SMSTemplateLabel:          "Mensaje preconfigurado",
		SMSCustomOption:           "Mensaje personalizado",
		SMSInfoOption:             "Teltonika · consultar información",
		SMSGPSOption:              "Teltonika · consultar posición GPS",
		SMSMessageLabel:           "Texto del mensaje",
		SMSHistoryTitle:           "Historial de mensajes",
		SMSHistoryEmpty:           "No hay mensajes enviados para esta SIM.",
		SMSHistoryLoading:         "Cargando historial…",
		SMSSendButton:             "Enviar SMS",
		SMSSentNotice:             "El SMS fue aceptado por 1GLOBAL.",
		SMSSendError:              "No se pudo enviar el SMS. Revisa el mensaje y el servicio de la SIM.",
		SIMHistoryPageTtl:         "Mensajes SMS",
		SIMHistoryPageSub:         "Mensajes enviados y recibidos por las SIM de este servidor.",
		SIMHistorySearch:          "Buscar",
		SIMHistorySearchHint:      "Dispositivo, ICCID, mensaje o estado",
		SIMHistoryDateCol:         "Fecha",
		SIMHistoryDirCol:          "Tipo",
		SIMHistoryReceived:        "Recibido",
		SIMHistorySent:            "Enviado",
		SIMHistoryMessageCol:      "Mensaje",
		SMSResendBtn:              "Reenviar",
		SMSHistoryUnavailable:     "No se pudo consultar el historial de mensajes.",
		ConnectivitySection:       "Proveedor de SIM",
		ConnectivityIntro:         "Configura la cuenta de conectividad de este usuario. La credencial se valida y se almacena cifrada.",
		ConnectivityActive:        "Proveedor configurado",
		ConnectivityMissing:       "No hay proveedor de SIM configurado.",
		ConnectivityProviderLabel: "Proveedor",
		ConnectivityTokenLabel:    "API key",
		ConnectivityTokenHint:     "La key no volverá a mostrarse después de guardarla.",
		ConnectivitySave:          "Guardar proveedor",
		ConnectivityRemove:        "Quitar proveedor",
		SIMsPageTtl:               "SIMs",
		SIMsPageSub:               "Inventario directo del proveedor, independiente de Traccar.",
		SIMsLoading:               "Cargando inventario y consumo histórico de SIMs…",
		SIMsEmpty:                 "El proveedor no devolvió SIMs.",
		SIMLabelCol:               "Etiqueta",
		SIMActivatedCol:           "Activada",
		SIMInventoryUnavailable:   "No se pudo consultar el inventario de SIMs.",
		SIMActionsCol:             "Acciones",
		SIMActivateBtn:            "Activar",
		SIMSuspendBtn:             "Suspender",
		SIMActivateConfirm:        "¿Activar la SIM %s?",
		SIMSuspendConfirm:         "¿Suspender la SIM %s?",
		SIMStatusUpdated:          "Estado actualizado.",
		SIMStatusFailed:           "No se pudo cambiar el estado de la SIM.",
		SIMStatusUnavailable:      "El proveedor no permite cambiar el estado de las SIMs.",
		SIMStatusInvalid:          "SIM o estado no válido.",
		SIMRefreshBtn:             "Actualizar ahora",
		SIMRefreshing:             "Actualizando inventario…",
		SIMRefreshFailed:          "No se pudo actualizar el inventario.",
		GoToSettings:              "Ir a Ajustes",
		LogoutButton:              "Cerrar sesión",
		SignedInAs:                "Sesión iniciada como",
		StatAccounts:              "Cuentas",
		StatActive:                "Al corriente",
		StatOverdue:               "Vencidas",
		ColAccount:                "Cuenta",
		ColEmail:                  "Correo",
		ColDevices:                "Dispositivos",
		ColAmount:                 "Cobro",
		ColStatus:                 "Estado",
		ColNextDue:                "Próximo corte",
		ColDaysLeft:               "Faltan",
		EmptyState:                "Aún no hay cuentas sincronizadas. Se sincronizan automáticamente cada pocos minutos.",
		PayButton:                 "Registrar pago",
		NoSubscription:            "sin configurar",
		ConfigureLabel:            "Configurar cobro",
		ResetPeriodLabel:          "Reiniciar periodo",
		ResetPeriodConfirm:        "¿Recalcular el próximo corte de esta cuenta? Esto no registra pagos ni liquida remisiones pendientes.",
		AmountLabel:               "Monto",
		CurrencyLabel:             "Moneda",
		PeriodLabel:               "Días por período",
		DueDateLabel:              "Próxima fecha de corte",
		SaveButton:                "Guardar",
		DaysLeftFmt:               "%d días",
		DaysOverdueFmt:            "vencida hace %d días",

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
		MenuButton:         "Menú",
		CloseButton:        "Cerrar",
		SortLabel:          "Ordenar por",
		TotalsLabel:        "Totales",
		QuantityLabel:      "Cant.",
		LineAmountLabel:    "Importe",
		AddLineButton:      "+ agregar línea",
		RemoveLineButton:   "Quitar línea",
		MonthlyLineOption:  "— mensualidad —",
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

		NavConcepts:     "Conceptos",
		ConceptsPageTtl: "Conceptos de cobro",
		ConceptsEmpty:   "Todavía no hay conceptos registrados. Da de alta uno para poder asociarlo a los pagos.",

		MonthNames: [12]string{"Enero", "Febrero", "Marzo", "Abril", "Mayo", "Junio",
			"Julio", "Agosto", "Septiembre", "Octubre", "Noviembre", "Diciembre"},
		IncomeByConcept:   "Ingresos por concepto",
		YearLabel:         "Año",
		MonthLabel:        "Mes",
		AllMonthsOption:   "Todo el año",
		GroupByDay:        "Por día",
		GroupByMonth:      "Por mes",
		ChargesCountLbl:   "Cobros",
		TopConceptLabel:   "Concepto principal",
		NoIncomeInPeriod:  "No hay ingresos registrados en este periodo.",
		ShareLabel:        "Participación",
		NewConceptButton:  "Nuevo concepto",
		NewConceptTitle:   "Nuevo concepto",
		EditConceptTitle:  "Editar concepto",
		ConceptLabel:      "Concepto",
		ConceptNameLabel:  "Nombre",
		SlugLabel:         "Slug / Clave",
		SlugHint:          "Identificador único (minúsculas, sin acentos ni espacios). Se genera automáticamente si se deja en blanco.",
		RecurringLabel:    "Recurrente",
		RecurringYes:      "Sí",
		RecurringNo:       "No",
		NoConceptOption:   "— sin concepto —",
		ColConcept:        "Concepto",
		OneOffChargeFmt:   "%s — cargo único de %s (no renueva la mensualidad)",
		OneOffBadge:       "cargo único",
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
		ToggleEmails:   "Ocultar/Censurar correos",

		NavSettings:     "Ajustes",
		SettingsPageTtl: "Ajustes de facturación",
		SettingsIntro:   "Estos valores se usan como punto de partida al configurar una cuenta nueva. Cambiarlos no afecta las suscripciones ya configuradas.",
		SettingsSaved:   "Ajustes guardados.",
		DefaultsSection: "Valores por defecto",
		DisplaySection:  "Presentación",
		HideMirrorLabel: "Ocultar cuentas espejo",
		HideMirrorHint:  "Traccar crea usuarios temporales al compartir un equipo (su correo lleva dos puntos y el ID del equipo). No son clientes y no se cobran.",

		RemissionsPageTtl:    "Remisiones",
		NavRemissions:        "Remisiones",
		RemissionPendingLbl:  "Pendiente",
		RemissionPaidLabel:   "Pagada",
		RemissionCanceledLbl: "Cancelada",
		RemissionAccountCol:  "Cliente",
		RemissionPeriodCol:   "Periodo",
		RemissionDevicesCol:  "Equipos",
		RemissionAmountCol:   "Importe",
		RemissionStatusCol:   "Estado",
		RemissionMarkPaidBtn: "Pagada",
		RemissionCancelBtn:   "Cancelar",
		RemissionEmpty:       "No hay remisiones con este filtro.",
		RemissionPendingSum:  "Por cobrar",
		RemissionUnbilledFmt: "%d cuenta(s) activa(s) sin suscripción: no se les generó remisión.",
		RemissionIntro:       "Se generan al inicio de cada periodo con los equipos que tenía la cuenta ese día. El importe queda congelado.",

		ConnectionSection: "Conexión con Traccar",
		TokenIntro:        "La cookie de tu sesión caduca. Sin un token, el sistema deja de poder suspender morosos en cuanto eso pasa, y no avisa.",
		TokenActiveLabel:  "Token activo",
		TokenMissingLabel: "Sin token",
		TokenMissingHint:  "Ahorita las tareas en segundo plano dependen de que entres seguido a la web.",
		TokenPasteLabel:   "Pegar un token de Traccar",
		TokenPasteHint:    "Se verifica contra tu servidor antes de guardarlo. Solo se muestra el principio y el final: no se vuelve a enseñar completo.",
		TokenGenerateBtn:  "Generar automáticamente",
		TokenRemoveBtn:    "Quitar token",
		TokenSaveBtn:      "Guardar token",
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

		NavExpenses:     "Retiros",
		ExpensesPageTtl: "Retiros",

		CategoryHint: "Ej. Pago a instalador",
		DefaultCategories: []string{
			"Pago a instalador", "Compra de equipo", "Comisión", "Gasolina",
			"Renta", "Sueldos", "Herramienta", "Otro",
		},
		NavAppointments:     "Agendas",
		AppointmentsPageTtl: "Agendas",
		AppointmentsEmpty:   "Todavía no hay visitas agendadas.",
		NewAppointmentBtn:   "Nueva agenda",
		NewAppointmentTtl:   "Nueva agenda",
		EditAppointmentTtl:  "Editar agenda",
		ClientLabel:         "Cliente",
		ContactLabel:        "Contacto",
		ContactHint:         "Varios teléfonos separados por comas",
		UnitLabel:           "Unidad",
		AddressLabel:        "Dirección",
		TimeWindowLabel:     "Horario",
		TimeWindowHint:      "Por ejemplo: 1 o 2 pm",
		AltasLabel:          "Altas",
		CostLabel:           "Costo",
		OutcomeLabel:        "Resultado",
		VisitScheduled:      "Agendada",
		VisitDone:           "Cerrada",
		VisitCanceled:       "Cancelada",
		CloseVisitBtn:       "Cerrar",
		CloseVisitTtl:       "Cerrar agenda",
		CancelVisitBtn:      "Cancelar visita",
		CancelVisitTtl:      "Cancelar agenda",
		ReopenVisitBtn:      "Reabrir",
		DeleteVisitTtl:      "Eliminar agenda",
		WhatsAppButton:      "WhatsApp",
		WhatsAppWebButton:   "WhatsApp Web",
		WhatsAppMsgFmt:      "Hola %s, le confirmamos su visita el %s en el horario %s.",
		WhatsAppUnitFmt:     "Unidad: %s.",
		WhatsAppAddrFmt:     "Dirección: %s",
		LateBadge:           "Atrasada",
		PendingVisits:       "Agendadas",
		DoneVisits:          "Cerradas",
		PendingAmount:       "Por cobrar",
		FilterOpen:          "Abiertas",
		FilterDone:          "Cerradas",
		FilterCanceled:      "Canceladas",
		ExpensesEmpty:       "Sin retiros registrados.",
		NoExpensesHere:      "No hay retiros en este periodo.",
		NewExpenseButton:    "Registrar retiro",
		NewExpenseTitle:     "Registrar retiro",
		EditExpenseTitle:    "Editar retiro",
		DeleteExpenseTtl:    "Eliminar retiro",
		CategoryLabel:       "Categoría",
		TotalWithdrawn:      "Total retiros",
		NetTotal:            "Total neto",
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

		DashboardTitle:            "Dashboard",
		NavDevices:                "Devices",
		NavSIMs:                   "SIMs",
		NavSIMHistory:             "SMS",
		DevicesPageTtl:            "Device data usage",
		DevicesLoading:            "Loading devices and usage in the background…",
		DevicesLoadingUsage:       "Completing SIM and usage data…",
		DeviceSearchLabel:         "Search",
		DeviceSearchHint:          "Name, IMEI, or ICCID",
		DevicesSortLabel:          "Sort",
		DevicesSortProvider:       "With SIM first",
		DevicesSortName:           "Name",
		DeviceNameCol:             "Device",
		DataAllowedCol:            "Data allowance",
		DataUsedCol:               "Data used",
		DataRemainingCol:          "Data remaining",
		DataRemainingCalculated:   "Balance calculated from the plan limit and the SIM's complete usage period.",
		ServicePackCol:            "Plan",
		TimeRemainingCol:          "Time remaining",
		UsagePeriodCol:            "Usage period",
		UsageUpdatedLabel:         "Checked",
		UsageUnavailable:          "No 1GLOBAL information is available for this IMEI.",
		UsageReportUnavailable:    "SIMs were identified, but 1GLOBAL could not generate the usage report.",
		DevicesEmpty:              "Traccar returned no devices.",
		DevicesFetchError:         "Could not fetch devices from Traccar.",
		ConnectivityNotConfigured: "Configure your provider and API key in Settings to fetch SIMs and usage.",
		DeviceProtocolFilterWarn:  "Could not fetch protocols; only recovery devices were hidden.",
		SMSAction:                 "Messages",
		SMSDialogTitle:            "Send message to device",
		SMSTemplateLabel:          "Preset message",
		SMSCustomOption:           "Custom message",
		SMSInfoOption:             "Teltonika · request information",
		SMSGPSOption:              "Teltonika · request GPS position",
		SMSMessageLabel:           "Message text",
		SMSHistoryTitle:           "Message history",
		SMSHistoryEmpty:           "No messages have been sent to this SIM.",
		SMSHistoryLoading:         "Loading history…",
		SMSSendButton:             "Send SMS",
		SMSSentNotice:             "1GLOBAL accepted the SMS.",
		SMSSendError:              "Could not send the SMS. Check the message and SIM service.",
		SIMHistoryPageTtl:         "SMS messages",
		SIMHistoryPageSub:         "Messages sent to and received from the SIMs on this server.",
		SIMHistorySearch:          "Search",
		SIMHistorySearchHint:      "Device, ICCID, message, or status",
		SIMHistoryDateCol:         "Date",
		SIMHistoryDirCol:          "Type",
		SIMHistoryReceived:        "Received",
		SIMHistorySent:            "Sent",
		SIMHistoryMessageCol:      "Message",
		SMSResendBtn:              "Resend",
		SMSHistoryUnavailable:     "Could not fetch message history.",
		ConnectivitySection:       "SIM provider",
		ConnectivityIntro:         "Configure this user's connectivity account. The credential is validated and stored encrypted.",
		ConnectivityActive:        "Provider configured",
		ConnectivityMissing:       "No SIM provider is configured.",
		ConnectivityProviderLabel: "Provider",
		ConnectivityTokenLabel:    "API key",
		ConnectivityTokenHint:     "The key will not be displayed again after it is saved.",
		ConnectivitySave:          "Save provider",
		ConnectivityRemove:        "Remove provider",
		SIMsPageTtl:               "SIMs",
		SIMsPageSub:               "Direct provider inventory, independent from Traccar.",
		SIMsLoading:               "Loading SIM inventory and lifetime usage…",
		SIMsEmpty:                 "The provider returned no SIMs.",
		SIMLabelCol:               "Label",
		SIMActivatedCol:           "Activated",
		SIMInventoryUnavailable:   "Could not fetch SIM inventory.",
		SIMActionsCol:             "Actions",
		SIMActivateBtn:            "Activate",
		SIMSuspendBtn:             "Suspend",
		SIMActivateConfirm:        "Activate SIM %s?",
		SIMSuspendConfirm:         "Suspend SIM %s?",
		SIMStatusUpdated:          "Status updated.",
		SIMStatusFailed:           "Could not change the SIM status.",
		SIMStatusUnavailable:      "The provider does not support changing SIM status.",
		SIMStatusInvalid:          "Invalid SIM or status.",
		SIMRefreshBtn:             "Refresh now",
		SIMRefreshing:             "Refreshing inventory…",
		SIMRefreshFailed:          "Could not refresh the inventory.",
		GoToSettings:              "Go to Settings",
		LogoutButton:              "Sign out",
		SignedInAs:                "Signed in as",
		StatAccounts:              "Accounts",
		StatActive:                "Current",
		StatOverdue:               "Overdue",
		ColAccount:                "Account",
		ColEmail:                  "Email",
		ColDevices:                "Devices",
		ColAmount:                 "Billing",
		ColStatus:                 "Status",
		ColNextDue:                "Next due date",
		ColDaysLeft:               "Left",
		EmptyState:                "No accounts synced yet. They sync automatically every few minutes.",
		PayButton:                 "Record payment",
		NoSubscription:            "not set up",
		ConfigureLabel:            "Set up billing",
		ResetPeriodLabel:          "Reset period",
		ResetPeriodConfirm:        "Recalculate this account's next due date? This does not record payments or settle pending remissions.",
		AmountLabel:               "Amount",
		CurrencyLabel:             "Currency",
		PeriodLabel:               "Billing period (days)",
		DueDateLabel:              "Next due date",
		SaveButton:                "Save",
		DaysLeftFmt:               "%d days",
		DaysOverdueFmt:            "%d days overdue",

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
		MenuButton:         "Menu",
		CloseButton:        "Close",
		SortLabel:          "Sort by",
		TotalsLabel:        "Totals",
		QuantityLabel:      "Qty",
		LineAmountLabel:    "Amount",
		AddLineButton:      "+ add line",
		RemoveLineButton:   "Remove line",
		MonthlyLineOption:  "— monthly fee —",
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

		NavConcepts:     "Concepts",
		ConceptsPageTtl: "Billing concepts",
		ConceptsEmpty:   "No billing concepts registered yet. Create one to associate it with payments.",

		MonthNames: [12]string{"January", "February", "March", "April", "May", "June",
			"July", "August", "September", "October", "November", "December"},
		IncomeByConcept:  "Income by concept",
		YearLabel:        "Year",
		MonthLabel:       "Month",
		AllMonthsOption:  "Whole year",
		GroupByDay:       "By day",
		GroupByMonth:     "By month",
		ChargesCountLbl:  "Charges",
		TopConceptLabel:  "Top concept",
		NoIncomeInPeriod: "No income recorded in this period.",
		ShareLabel:       "Share",
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
		OneOffChargeFmt:  "%s — one-off charge of %s (does not renew subscription)",
		OneOffBadge:      "one-off charge",

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
		ToggleEmails:   "Hide/Censor emails",

		NavSettings:     "Settings",
		SettingsPageTtl: "Billing settings",
		SettingsIntro:   "These values are the starting point when configuring a new account. Changing them leaves existing subscriptions untouched.",
		SettingsSaved:   "Settings saved.",
		DefaultsSection: "Defaults",
		DisplaySection:  "Display",
		HideMirrorLabel: "Hide mirror accounts",
		HideMirrorHint:  "Traccar creates temporary users when a device is shared (their email carries a colon and the device ID). They are not customers and are never billed.",

		RemissionsPageTtl:    "Remissions",
		NavRemissions:        "Remissions",
		RemissionPendingLbl:  "Pending",
		RemissionPaidLabel:   "Paid",
		RemissionCanceledLbl: "Canceled",
		RemissionAccountCol:  "Customer",
		RemissionPeriodCol:   "Period",
		RemissionDevicesCol:  "Devices",
		RemissionAmountCol:   "Amount",
		RemissionStatusCol:   "Status",
		RemissionMarkPaidBtn: "Paid",
		RemissionCancelBtn:   "Cancel",
		RemissionEmpty:       "No remissions match this filter.",
		RemissionPendingSum:  "Outstanding",
		RemissionUnbilledFmt: "%d active account(s) with no subscription: no remission was issued for them.",
		RemissionIntro:       "Issued at the start of each period from the devices the account had that day. The amount is frozen.",

		ConnectionSection: "Traccar connection",
		TokenIntro:        "Your login cookie expires. Without a token, the system stops being able to suspend overdue accounts the moment it does, and says nothing.",
		TokenActiveLabel:  "Token active",
		TokenMissingLabel: "No token",
		TokenMissingHint:  "Background work currently depends on you signing in often enough.",
		TokenPasteLabel:   "Paste a Traccar token",
		TokenPasteHint:    "Checked against your server before it is saved. Only the first and last characters are ever shown again.",
		TokenGenerateBtn:  "Generate automatically",
		TokenRemoveBtn:    "Remove token",
		TokenSaveBtn:      "Save token",
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

		NavExpenses:     "Expenses",
		ExpensesPageTtl: "Expenses",

		CategoryHint: "e.g. Installer payment",
		DefaultCategories: []string{
			"Installer payment", "Equipment purchase", "Commission", "Fuel",
			"Rent", "Payroll", "Tools", "Other",
		},
		NavAppointments:     "Appointments",
		AppointmentsPageTtl: "Appointments",
		AppointmentsEmpty:   "No visits scheduled yet.",
		NewAppointmentBtn:   "New appointment",
		NewAppointmentTtl:   "New appointment",
		EditAppointmentTtl:  "Edit appointment",
		ClientLabel:         "Client",
		ContactLabel:        "Contact",
		ContactHint:         "Several phones separated by commas",
		UnitLabel:           "Vehicle",
		AddressLabel:        "Address",
		TimeWindowLabel:     "Time",
		TimeWindowHint:      "For example: 1 to 2 pm",
		AltasLabel:          "Installs",
		CostLabel:           "Cost",
		OutcomeLabel:        "Outcome",
		VisitScheduled:      "Scheduled",
		VisitDone:           "Closed",
		VisitCanceled:       "Canceled",
		CloseVisitBtn:       "Close",
		CloseVisitTtl:       "Close appointment",
		CancelVisitBtn:      "Cancel visit",
		CancelVisitTtl:      "Cancel appointment",
		ReopenVisitBtn:      "Reopen",
		DeleteVisitTtl:      "Delete appointment",
		WhatsAppButton:      "WhatsApp",
		WhatsAppWebButton:   "WhatsApp Web",
		WhatsAppMsgFmt:      "Hi %s, confirming your visit on %s between %s.",
		WhatsAppUnitFmt:     "Vehicle: %s.",
		WhatsAppAddrFmt:     "Address: %s",
		LateBadge:           "Late",
		PendingVisits:       "Scheduled",
		DoneVisits:          "Closed",
		PendingAmount:       "To collect",
		FilterOpen:          "Open",
		FilterDone:          "Closed",
		FilterCanceled:      "Canceled",
		ExpensesEmpty:       "No expenses recorded yet.",
		NoExpensesHere:      "No expenses in this period.",
		NewExpenseButton:    "Record withdrawal",
		NewExpenseTitle:     "Record withdrawal",
		EditExpenseTitle:    "Edit withdrawal",
		DeleteExpenseTtl:    "Delete withdrawal",
		CategoryLabel:       "Category",
		TotalWithdrawn:      "Total withdrawn",
		NetTotal:            "Net total",
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

func (t uiStrings) appointmentStatusLabel(status billing.AppointmentStatus) string {
	switch status {
	case billing.AppointmentScheduled:
		return t.VisitScheduled
	case billing.AppointmentDone:
		return t.VisitDone
	case billing.AppointmentCanceled:
		return t.VisitCanceled
	default:
		return string(status)
	}
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
