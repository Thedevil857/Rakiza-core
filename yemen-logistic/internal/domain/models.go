// Package domain contains the pure, framework-agnostic business entities of
// the Ministry of Industry and Trade logistics platform. Nothing in this
// package imports net/http, database/sql, or any transport/persistence
// library — that isolation is the entire point of Clean/Hexagonal
// Architecture: the domain must be compilable and testable with zero
// external dependencies.
package domain

import "time"


// Merchant


// MerchantStatus is a closed set of states a merchant account can be in.
// It mirrors the PostgreSQL ENUM `merchant_status` exactly, character for
// character, so DB values can be scanned directly into this type.
type MerchantStatus string

const (
	MerchantStatusActive    MerchantStatus = "Active"
	MerchantStatusPending   MerchantStatus = "Pending"
	MerchantStatusSuspended MerchantStatus = "Suspended"
)

// IsValid reports whether the MerchantStatus is one of the known enum values.
// Used at the service boundary to reject malformed input before it ever
// reaches a SQL statement.
func (s MerchantStatus) IsValid() bool {
	switch s {
	case MerchantStatusActive, MerchantStatusPending, MerchantStatusSuspended:
		return true
	default:
		return false
	}
}

// IsActive reports whether this merchant's account is fully activated.
// Financial and service-request actions (customs permit submission,
// wallet top-up requests, etc.) should check this and refuse a Pending
// account server-side a disabled button in the UI is not enforcement,
// only a hint; the account is genuinely gated here, at the data layer.
func (s MerchantStatus) IsActive() bool {
	return s == MerchantStatusActive
}

// MerchantTradeNature is a closed set mirroring the PostgreSQL ENUM
// `merchant_trade_nature`, collected during the onboarding wizard's
// Business step.
type MerchantTradeNature string

const (
	
	
	MerchantTradeNatureImportExport  MerchantTradeNature = "import_export"
	MerchantTradeNatureImport        MerchantTradeNature = "import"
	MerchantTradeNatureExport        MerchantTradeNature = "export"
	MerchantTradeNatureWholesale     MerchantTradeNature = "wholesale"
	MerchantTradeNatureManufacturing MerchantTradeNature = "manufacturing"
)

// IsValid reports whether the MerchantTradeNature is one of the known enum
// values.
func (t MerchantTradeNature) IsValid() bool {
	switch t {
	case  
		MerchantTradeNatureImportExport, MerchantTradeNatureImport, MerchantTradeNatureExport,
		MerchantTradeNatureWholesale, MerchantTradeNatureManufacturing:
		return true
	default:
		return false
	}
}

type MerchantDocument struct {
	Type       string    `json:"type"`
	URL        string    `json:"url"`
	UploadedAt time.Time `json:"uploadedAt"`
}

type Merchant struct {
	ID                   string         `json:"id" db:"id"`
	Username             string         `json:"username" db:"username"`
	PasswordHash         string         `json:"-" db:"password_hash"` // never serialized to JSON
	CompanyName          string         `json:"company_name" db:"company_name"`
	CompanyNameAr        string         `json:"company_name_ar" db:"company_name_ar"`
	CompanyNameEn        string         `json:"company_name_en" db:"company_name_en"`
	CommercialRegisterNo string         `json:"commercial_register_no" db:"commercial_register_no"`
	LicenseNumber        string         `json:"license_number" db:"license_number"`
	Phone                string         `json:"phone" db:"phone"`
	WalletBalance        float64        `json:"wallet_balance" db:"wallet_balance"`
	Status               MerchantStatus `json:"status" db:"status"`

	// --- Onboarding profile (Phase 2) ---
	OnboardingCompleted     bool                 `json:"onboarding_completed" db:"onboarding_completed"`
	TaxID                   *string              `json:"tax_id,omitempty" db:"tax_id"`
	AuthorizedSignatoryName *string              `json:"authorized_signatory_name,omitempty" db:"authorized_signatory_name"`
	AuthorizedSignatoryID   *string              `json:"authorized_signatory_id,omitempty" db:"authorized_signatory_id"`
	AddressLine             *string              `json:"address_line,omitempty" db:"address_line"`
	City                    *string              `json:"city,omitempty" db:"city"`
	GPSLat                  *float64             `json:"gps_lat,omitempty" db:"gps_lat"`
	GPSLng                  *float64             `json:"gps_lng,omitempty" db:"gps_lng"`
	TradeNature             *MerchantTradeNature `json:"trade_nature,omitempty" db:"trade_nature"`
	PrimaryGoodsCategory    *string              `json:"primary_goods_category,omitempty" db:"primary_goods_category"`
	DefaultCountryOfOrigin  *string              `json:"default_country_of_origin,omitempty" db:"default_country_of_origin"`
	DefaultPortOfEntry      *string              `json:"default_port_of_entry,omitempty" db:"default_port_of_entry"`
	DefaultExporterName     *string              `json:"default_exporter_name,omitempty" db:"default_exporter_name"`
	LegalDocuments          []MerchantDocument   `json:"legal_documents" db:"legal_documents"`

	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// MerchantOnboardingInput is the input DTO for completing a merchant's
// first-time onboarding profile. It is a plain struct (not Merchant itself)
// so the onboarding handler has a narrow, explicit surface of exactly what
// the wizard is allowed to set, independent of Merchant's full field set.
type MerchantOnboardingInput struct {
	TaxID                   string              `json:"tax_id" form:"taxId"`
	AuthorizedSignatoryName string              `json:"authorized_signatory_name" form:"authorizedSignatoryName"`
	AuthorizedSignatoryID   string              `json:"authorized_signatory_id" form:"authorizedSignatoryID"`
	CompanyNameAr           string              `json:"company_name_ar" form:"companyNameAr"`
	CompanyNameEn           string              `json:"company_name_en" form:"companyNameEn"`
	AddressLine             string              `json:"address_line" form:"addressLine"`
	City                    string              `json:"city" form:"city"`
	GPSLat                  float64             `json:"gps_lat" form:"gpsLat"`
	GPSLng                  float64             `json:"gps_lng" form:"gpsLng"`
	TradeNature             MerchantTradeNature `json:"trade_nature" form:"tradeNature"`
	PrimaryGoodsCategory    string              `json:"primary_goods_category" form:"primaryGoodsCategory"`
	DefaultCountryOfOrigin  string              `json:"default_country_of_origin" form:"defaultCountryOfOrigin"`
	DefaultPortOfEntry      string              `json:"default_port_of_entry" form:"defaultPortOfEntry"`
	DefaultExporterName     string              `json:"default_exporter_name" form:"defaultExporterName"`
	CommercialRegisterNo    string              `json:"commercial_register_no" form:"commercialRegisterNo"`
	Phone                   string              `json:"phone" form:"phone"`
	LegalDocuments          []MerchantDocument  `json:"legal_documents"`
}


// Staff (internal Ministry accounts: admin / officer)


// StaffRole is a closed set mirroring the PostgreSQL ENUM `staff_role`.
// Values are lowercase to match the existing JWT "role" claim strings used
// throughout the system ("merchant", "admin", "officer") without requiring
// any case translation at the auth boundary.
type StaffRole string

const (
	StaffRoleAdmin   StaffRole = "admin"
	StaffRoleOfficer StaffRole = "officer"
)

// IsValid reports whether the StaffRole is one of the known enum values.
func (r StaffRole) IsValid() bool {
	switch r {
	case StaffRoleAdmin, StaffRoleOfficer:
		return true
	default:
		return false
	}
}

// StaffStatus is a closed set mirroring the PostgreSQL ENUM `staff_status`.
type StaffStatus string

const (
	StaffStatusActive    StaffStatus = "Active"
	StaffStatusSuspended StaffStatus = "Suspended"
)

// IsValid reports whether the StaffStatus is one of the known enum values.
func (s StaffStatus) IsValid() bool {
	switch s {
	case StaffStatusActive, StaffStatusSuspended:
		return true
	default:
		return false
	}
}

// Staff represents an internal Ministry account -- either the Minister
// (JWT/DB role "admin") or a checkpoint Officer (role "officer").
type Staff struct {
	ID           string    `json:"id" db:"id"`
	Username     string    `json:"username" db:"username"`
	PasswordHash string    `json:"-" db:"password"`
	Role         StaffRole `json:"role" db:"role"`
	NameAr       string    `json:"name_ar" db:"name_ar"`
	NameEn       string    `json:"name_en" db:"name_en"`
	TitleAr      string    `json:"title_ar" db:"title_ar"`
	TitleEn      string    `json:"title_en" db:"title_en"`
}

// -----------------------------------------------------------------------------
// Shipment
// -----------------------------------------------------------------------------

// ShipmentStatus is a closed set of states a shipment can be in, mirroring
// the PostgreSQL ENUM `shipment_status`.
type ShipmentStatus string

const (
	ShipmentStatusCreated          ShipmentStatus = "Created"
	ShipmentStatusShipped          ShipmentStatus = "Shipped"
	ShipmentStatusCustomsClearance ShipmentStatus = "Customs_Clearance"
	ShipmentStatusArrived          ShipmentStatus = "Arrived"
)

// IsValid reports whether the ShipmentStatus is one of the known enum values.
func (s ShipmentStatus) IsValid() bool {
	switch s {
	case ShipmentStatusCreated, ShipmentStatusShipped, ShipmentStatusCustomsClearance, ShipmentStatusArrived:
		return true
	default:
		return false
	}
}

// nextAllowedShipmentStatus encodes the forward-only lifecycle a shipment is
// permitted to move through. A shipment can never move backwards (e.g. from
// Arrived back to Shipped) — that would indicate data corruption or fraud.
var nextAllowedShipmentStatus = map[ShipmentStatus]ShipmentStatus{
	ShipmentStatusCreated:          ShipmentStatusShipped,
	ShipmentStatusShipped:          ShipmentStatusCustomsClearance,
	ShipmentStatusCustomsClearance: ShipmentStatusArrived,
}

// CanTransitionTo reports whether moving from the current status to target
// is a legal forward transition in the shipment lifecycle.
func (s ShipmentStatus) CanTransitionTo(target ShipmentStatus) bool {
	next, ok := nextAllowedShipmentStatus[s]
	return ok && next == target
}

// Shipment represents a single consignment of cargo moving through the
// Ministry's supply chain, from creation through customs to arrival.
type Shipment struct {
	ID             string         `json:"id" db:"id"`
	MerchantID     string         `json:"merchant_id" db:"merchant_id"`
	TrackingNumber string         `json:"tracking_number" db:"tracking_number"`
	CurrentStatus  ShipmentStatus `json:"current_status" db:"current_status"`
	CargoDetails   string         `json:"cargo_details" db:"cargo_details"`
	TotalWeight    float64        `json:"total_weight" db:"total_weight"`
	CreatedAt      time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at" db:"updated_at"`
}

// -----------------------------------------------------------------------------
// Invoice
// -----------------------------------------------------------------------------

// PaymentStatus is a closed set of states an invoice's payment can be in,
// mirroring the PostgreSQL ENUM `payment_status`.
type PaymentStatus string

const (
	PaymentStatusPending PaymentStatus = "Pending"
	PaymentStatusPaid    PaymentStatus = "Paid"
	PaymentStatusFailed  PaymentStatus = "Failed"
)

// IsValid reports whether the PaymentStatus is one of the known enum values.
func (s PaymentStatus) IsValid() bool {
	switch s {
	case PaymentStatusPending, PaymentStatusPaid, PaymentStatusFailed:
		return true
	default:
		return false
	}
}

// Invoice represents the financial record for a single shipment.
type Invoice struct {
	ID                    string        `json:"id" db:"id"`
	ShipmentID            string        `json:"shipment_id" db:"shipment_id"`
	MerchantID            string        `json:"merchant_id" db:"merchant_id"`
	BaseItemCost          float64       `json:"base_item_cost" db:"base_item_cost"`
	FreightCharges        float64       `json:"freight_charges" db:"freight_charges"`
	EstimatedCustomsDuty  float64       `json:"estimated_customs_duty" db:"estimated_customs_duty"`
	DomesticTransportCost float64       `json:"domestic_transport_cost" db:"domestic_transport_cost"`
	TotalPaid             float64       `json:"total_paid" db:"total_paid"`
	PaymentStatus         PaymentStatus `json:"payment_status" db:"payment_status"`
	QRTokenString         string        `json:"qr_token_string,omitempty" db:"qr_token_string"`
	TruckLicense          string        `json:"truck_license,omitempty" db:"truck_license"`
	CreatedAt             time.Time     `json:"created_at" db:"created_at"`
	UpdatedAt             time.Time     `json:"updated_at" db:"updated_at"`
}

// GrandTotal computes the total payable amount for the invoice.
func (i Invoice) GrandTotal() float64 {
	return i.BaseItemCost + i.FreightCharges + i.EstimatedCustomsDuty + i.DomesticTransportCost
}

// OutstandingBalance returns the amount still owed on the invoice.
func (i Invoice) OutstandingBalance() float64 {
	balance := i.GrandTotal() - i.TotalPaid
	if balance < 0 {
		return 0
	}
	return balance
}

// -----------------------------------------------------------------------------
// Checkpoint
// -----------------------------------------------------------------------------

// Checkpoint represents a physical inspection post staffed by a Ministry inspector.
type Checkpoint struct {
	ID           string    `json:"id" db:"id"`
	LocationName string    `json:"location_name" db:"location_name"`
	InspectorID  string    `json:"inspector_id" db:"inspector_id"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// -----------------------------------------------------------------------------
// Permit
// -----------------------------------------------------------------------------

// PermitStatus mirrors the PostgreSQL ENUM `permit_status`.
type PermitStatus string

const (
	PermitStatusPending  PermitStatus = "Pending"
	PermitStatusCleared  PermitStatus = "Cleared"
	PermitStatusRejected PermitStatus = "Rejected"
)

// IsValid reports whether the PermitStatus is one of the known enum values.
func (s PermitStatus) IsValid() bool {
	switch s {
	case PermitStatusPending, PermitStatusCleared, PermitStatusRejected:
		return true
	default:
		return false
	}
}

// CategoryDutyRates dynamic mapping matching the frontend categories.
var CategoryDutyRates = map[string]float64{
	"food":        0.02, // 2% للمواد الغذائية الأساسية
	"medical":     0.00, // 0% للمستلزمات الطبية
	"building":    0.05, // 5% لمواد البناء
	"electronics": 0.10, // 10% للإلكترونيات
	"clothes":     0.07, // 7% للملابس والمنسوجات
}

// DefaultDutyRate serves as a fallback rate (5%).
const DefaultDutyRate = 0.05

// Permit represents a single cargo customs clearance request submitted by a
// merchant through the Merchant Portal.
type Permit struct {
	ID             string       `json:"id" db:"id"`
	MerchantID     string       `json:"merchant_id" db:"merchant_id"`
	Type           string       `json:"type" db:"type"`
	CategoryID     string       `json:"category_id,omitempty" db:"category_id"`
	Weight         float64      `json:"weight" db:"weight"`
	Value          float64      `json:"value" db:"value"`
	Duty           float64      `json:"duty" db:"duty"`
	TruckNumber    string       `json:"truck_number,omitempty" db:"truck_number"`
	ProductionDate *time.Time   `json:"production_date,omitempty" db:"production_date"`
	ExpiryDate     *time.Time   `json:"expiry_date,omitempty" db:"expiry_date"`
	Status         PermitStatus `json:"status" db:"status"`
	CreatedAt      time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at" db:"updated_at"`
}

// CalculateDuty returns the customs duty owed for a declared cargo value,
// calculated dynamically based on category_id or fallback to DefaultDutyRate.
func CalculateDuty(value float64, categoryID string) float64 {
	rate, found := CategoryDutyRates[categoryID]
	if !found {
		rate = DefaultDutyRate
	}
	return value * rate
}

// -----------------------------------------------------------------------------
// PermitClearance
// -----------------------------------------------------------------------------

// PermitClearanceDecision is a closed set mirroring the PostgreSQL ENUM
// `permit_clearance_decision`.
type PermitClearanceDecision string

const (
	PermitClearanceDecisionPassed   PermitClearanceDecision = "Passed"
	PermitClearanceDecisionRejected PermitClearanceDecision = "Rejected"
)

// IsValid reports whether the PermitClearanceDecision is one of the known
// enum values.
func (d PermitClearanceDecision) IsValid() bool {
	switch d {
	case PermitClearanceDecisionPassed, PermitClearanceDecisionRejected:
		return true
	default:
		return false
	}
}

// PermitClearance represents a single officer decision made at a physical checkpoint.
type PermitClearance struct {
	ID        string                  `json:"id" db:"id"`
	PermitID  string                  `json:"permit_id" db:"permit_id"`
	OfficerID string                  `json:"officer_id" db:"officer_id"`
	Decision  PermitClearanceDecision `json:"decision" db:"decision"`
	DecidedAt time.Time               `json:"decided_at" db:"decided_at"`
	SyncedAt  time.Time               `json:"synced_at" db:"synced_at"`
	CreatedAt time.Time               `json:"created_at" db:"created_at"`
}