package models

import (
	"fmt"
	"time"

	"github.com/gobuffalo/pop"
	"github.com/gobuffalo/validate"
	"github.com/gobuffalo/validate/validators"
	"github.com/gofrs/uuid"
	"github.com/pkg/errors"

	"github.com/transcom/mymove/pkg/unit"
)

// ShipmentLineItemStatus represents the status of a line item's lifecycle
type ShipmentLineItemStatus string

// ShipmentLineItemLocation represents the location of the line item
type ShipmentLineItemLocation string

const (
	// ShipmentLineItemStatusSUBMITTED captures enum value "SUBMITTED"
	ShipmentLineItemStatusSUBMITTED ShipmentLineItemStatus = "SUBMITTED"
	// ShipmentLineItemStatusAPPROVED captures enum value "APPROVED"
	ShipmentLineItemStatusAPPROVED ShipmentLineItemStatus = "APPROVED"
	// ShipmentLineItemStatusCONDITIONALLYAPPROVED captures enum value "CONDITIONALLY_APPROVED"
	ShipmentLineItemStatusCONDITIONALLYAPPROVED ShipmentLineItemStatus = "CONDITIONALLY_APPROVED"
	// ShipmentLineItemLocationORIGIN captures enum value "ORIGIN"
	ShipmentLineItemLocationORIGIN ShipmentLineItemLocation = "ORIGIN"
	// ShipmentLineItemLocationDESTINATION captures enum value "DESTINATION"
	ShipmentLineItemLocationDESTINATION ShipmentLineItemLocation = "DESTINATION"
	// ShipmentLineItemLocationNEITHER captures enum value "NEITHER"
	ShipmentLineItemLocationNEITHER ShipmentLineItemLocation = "NEITHER"
)

// ShipmentLineItem is an object representing a line item in a pre-approval request
type ShipmentLineItem struct {
	ID         uuid.UUID `json:"id" db:"id"`
	ShipmentID uuid.UUID `json:"shipment_id" db:"shipment_id"`
	Shipment   Shipment  `belongs_to:"shipments"`

	Tariff400ngItemID uuid.UUID                `json:"tariff400ng_item_id" db:"tariff400ng_item_id"`
	Tariff400ngItem   Tariff400ngItem          `belongs_to:"tariff400ng_items"`
	Location          ShipmentLineItemLocation `json:"location" db:"location"`

	// Enter numbers only, no symbols or units. Examples:
	// Crating: enter "47.4" for crate size of 47.4 cu. ft.
	// 3rd-party service: enter "1299.99" for cost of $1,299.99.
	// Bulky item: enter "1" for a single item.
	// Time - Free form text
	Quantity1           unit.BaseQuantity          `json:"quantity_1" db:"quantity_1"`
	Quantity2           unit.BaseQuantity          `json:"quantity_2" db:"quantity_2"`
	Notes               string                     `json:"notes" db:"notes"`
	Status              ShipmentLineItemStatus     `json:"status" db:"status"`
	InvoiceID           *uuid.UUID                 `json:"invoice_id" db:"invoice_id"`
	Invoice             Invoice                    `belongs_to:"invoices"`
	EstimateAmountCents *unit.Cents                `json:"estimate_amount_cents" db:"estimate_amount_cents"`
	ActualAmountCents   *unit.Cents                `json:"actual_amount_cents" db:"actual_amount_cents"`
	AmountCents         *unit.Cents                `json:"amount_cents" db:"amount_cents"`
	AppliedRate         *unit.Millicents           `json:"applied_rate" db:"applied_rate"`
	SubmittedDate       time.Time                  `json:"submitted_date" db:"submitted_date"`
	ApprovedDate        time.Time                  `json:"approved_date" db:"approved_date"`
	ItemDimensionsID    *uuid.UUID                 `json:"item_dimensions_id" db:"item_dimensions_id"`
	ItemDimensions      ShipmentLineItemDimensions `belongs_to:"shipment_line_item_dimensions"`
	CrateDimensionsID   *uuid.UUID                 `json:"crate_dimensions_id" db:"crate_dimensions_id"`
	CrateDimensions     ShipmentLineItemDimensions `belongs_to:"shipment_line_item_dimensions"`
	Description         *string                    `json:"description" db:"description"`
	Reason              *string                    `json:"reason" db:"reason"`
	Date                *time.Time                 `json:"date" db:"date"`
	Time                *string                    `json:"time" db:"time"`
	AddressID           *uuid.UUID                 `json:"address_id" db:"address_id"`
	Address             Address                    `belongs_to:"addresses"`
	CreatedAt           time.Time                  `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time                  `json:"updated_at" db:"updated_at"`
}

// ShipmentLineItems is not required by pop and may be deleted
type ShipmentLineItems []ShipmentLineItem

func StorageInTransitLineItemCodes() []string {
	return []string{
		"185A",
		"185B",
		"210A",
		"210B",
		"210C",
		"210F",
	}
}

func BaseShipmentLineItemCodes() []string {
	return []string{
		"LHS",
		"135A",
		"135B",
		"105A",
		"105C",
		"16A",
	}
}

// Validate gets run every time you call a "pop.Validate*" (pop.ValidateAndSave, pop.ValidateAndCreate, pop.ValidateAndUpdate) method.
// This method is not required and may be deleted.
func (s *ShipmentLineItem) Validate(tx *pop.Connection) (*validate.Errors, error) {
	if s == nil {
		return validate.NewErrors(), nil
	}

	validStatuses := []string{
		string(ShipmentLineItemStatusSUBMITTED),
		string(ShipmentLineItemStatusAPPROVED),
		string(ShipmentLineItemStatusCONDITIONALLYAPPROVED),
	}

	validLocations := []string{
		string(ShipmentLineItemLocationORIGIN),
		string(ShipmentLineItemLocationDESTINATION),
		string(ShipmentLineItemLocationNEITHER),
	}

	return validate.Validate(
		&validators.StringInclusion{Field: string(s.Status), Name: "Status", List: validStatuses},
		&validators.StringInclusion{Field: string(s.Location), Name: "Locations", List: validLocations},
	), nil
}

// BeforeDestroy verifies that a ShipmentLineItem is in a state to be destroyed
func (s *ShipmentLineItem) BeforeDestroy(tx *pop.Connection) error {
	if s.InvoiceID != nil {
		return ErrDestroyForbidden
	}

	return nil
}

// AfterDestroy also destroys associated items in the dimensions table, if they exist
func (s *ShipmentLineItem) AfterDestroy(tx *pop.Connection) error {

	if s.ItemDimensionsID != nil {
		err := tx.Destroy(&s.ItemDimensions)
		if err != nil {
			return err
		}
	}
	if s.CrateDimensionsID != nil {
		err := tx.Destroy(&s.CrateDimensions)
		if err != nil {
			return err
		}
	}
	if s.AddressID != nil {
		err := tx.Destroy(&s.Address)
		if err != nil {
			return err
		}
	}

	return nil
}

// FetchLineItemsByShipmentID returns a list of line items by shipment_id
func FetchLineItemsByShipmentID(dbConnection *pop.Connection, shipmentID *uuid.UUID) ([]ShipmentLineItem, error) {
	var err error

	if shipmentID == nil {
		return nil, errors.Wrap(err, "Missing shipmentID")
	}

	shipmentLineItems := []ShipmentLineItem{}

	query := dbConnection.Where("shipment_id = ?", *shipmentID)

	err = query.Eager().All(&shipmentLineItems)
	if err != nil {
		return shipmentLineItems, errors.Wrap(err, "Fetch line items query failed")
	}

	return shipmentLineItems, err
}

// FetchApprovedPreapprovalRequestsByShipment fetches approved pre-approval requests for a shipment
func FetchApprovedPreapprovalRequestsByShipment(dbConnection *pop.Connection, shipment Shipment) ([]ShipmentLineItem, error) {
	var items []ShipmentLineItem

	sitCodes := StorageInTransitLineItemCodes()
	storageInTransitCodes := make([]interface{}, len(sitCodes))
	for i, t := range sitCodes {
		storageInTransitCodes[i] = t
	}

	query := dbConnection.Q().
		LeftJoin("tariff400ng_items", "shipment_line_items.tariff400ng_item_id=tariff400ng_items.id").
		Where("shipment_id = ?", shipment.ID).
		Where("status = ?", ShipmentLineItemStatusAPPROVED).
		Where("tariff400ng_items.requires_pre_approval = true").
		Where("code not in (?)", storageInTransitCodes...).
		Eager("Tariff400ngItem")

	err := query.All(&items)

	// Add the shipment model
	for i := 0; i < len(items); i++ {
		items[i].Shipment = shipment
	}

	return items, err
}

// FetchShipmentLineItemByID returns a shipment line item by id
func FetchShipmentLineItemByID(dbConnection *pop.Connection, shipmentLineItemID *uuid.UUID) (ShipmentLineItem, error) {
	var err error

	if shipmentLineItemID == nil {
		return ShipmentLineItem{}, errors.Wrap(err, "Missing shipmentLineItemID")
	}

	shipmentLineItem := ShipmentLineItem{}

	err = dbConnection.Eager().Find(&shipmentLineItem, shipmentLineItemID)
	if err != nil {
		return shipmentLineItem, errors.Wrap(err, "Shipment line items query failed")
	}

	return shipmentLineItem, err
}

// Approve marks the ShipmentLineItem request as Approved. Must be in a submitted state or Conditionally Approved state.
func (s *ShipmentLineItem) Approve() error {
	if !(s.Status == ShipmentLineItemStatusSUBMITTED || (s.Tariff400ngItem.Code == "35A" && s.Status == ShipmentLineItemStatusCONDITIONALLYAPPROVED)) {
		var logMsg = "func Approve(): Current ShipmentLineItem status is [" + string(s.Status) + "]"
		return errors.Wrap(ErrInvalidTransition, logMsg)
	}
	s.Status = ShipmentLineItemStatusAPPROVED
	if s.ApprovedDate.IsZero() {
		s.ApprovedDate = time.Now()
	}
	return nil
}

// ConditionallyApprove marks the ShipmentLineItem request as Conditionally Approved. Must be in a submitted state.
func (s *ShipmentLineItem) ConditionallyApprove() error {
	if !(s.Status == ShipmentLineItemStatusSUBMITTED || (s.Tariff400ngItem.Code == "35A" && s.Status == ShipmentLineItemStatusAPPROVED)) {
		var logMsg = "func ConditionallyApprove(): Current ShipmentLineItem status is [" + string(s.Status) + "]"
		return errors.Wrap(ErrInvalidTransition, logMsg)
	}
	s.Status = ShipmentLineItemStatusCONDITIONALLYAPPROVED
	if s.ApprovedDate.IsZero() {
		s.ApprovedDate = time.Now()
	}
	return nil
}

// FindBaseShipmentLineItem returns true if code is a Base Shipment Line Item
// otherwise, returns false
func FindBaseShipmentLineItem(code string) bool {
	for _, item := range BaseShipmentLineItemCodes() {
		if code == item {
			return true
		}
	}
	return false
}

// FindStorageInTransitShipmentLineItem returns true if code is a Storage in Transit Shipment Line Item
// otherwise, returns false
func FindStorageInTransitShipmentLineItem(code string) bool {
	for _, base := range StorageInTransitLineItemCodes() {
		if code == base {
			return true
		}
	}
	return false
}

// VerifyBaseShipmentLineItems checks that all of the expected base line items are in use, as they are mandatory for
// every shipment
func VerifyBaseShipmentLineItems(lineItems []ShipmentLineItem) error {
	m := make(map[string]int)
	for _, code := range BaseShipmentLineItemCodes() {
		m[code] = 0
	}

	for _, item := range lineItems {
		if !item.Tariff400ngItem.RequiresPreApproval && FindBaseShipmentLineItem(item.Tariff400ngItem.Code) {
			m[item.Tariff400ngItem.Code]++
		}
	}

	for code, count := range m {
		if count != 1 {
			return fmt.Errorf("incorrect count for Base Shipment Line Item %s expected count: 1, actual count: %d", code, count)
		}
	}

	return nil
}