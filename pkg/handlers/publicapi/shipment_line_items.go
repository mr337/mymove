package publicapi

import (
	"database/sql"
	"fmt"

	"github.com/go-openapi/runtime/middleware"
	"github.com/gofrs/uuid"
	"github.com/pkg/errors"

	"github.com/transcom/mymove/pkg/services"

	"go.uber.org/zap"

	"github.com/transcom/mymove/pkg/gen/apimessages"
	accessorialop "github.com/transcom/mymove/pkg/gen/restapi/apioperations/accessorials"
	"github.com/transcom/mymove/pkg/handlers"
	"github.com/transcom/mymove/pkg/models"
	"github.com/transcom/mymove/pkg/rateengine"
	"github.com/transcom/mymove/pkg/unit"
)

func payloadForShipmentLineItemModels(s []models.ShipmentLineItem) apimessages.ShipmentLineItems {
	payloads := make(apimessages.ShipmentLineItems, len(s))

	for i, acc := range s {
		payloads[i] = payloadForShipmentLineItemModel(&acc)
	}

	return payloads
}

func payloadForShipmentLineItemModel(s *models.ShipmentLineItem) *apimessages.ShipmentLineItem {
	if s == nil {
		return nil
	}

	return &apimessages.ShipmentLineItem{
		ID:                  *handlers.FmtUUID(s.ID),
		ShipmentID:          *handlers.FmtUUID(s.ShipmentID),
		Tariff400ngItem:     payloadForTariff400ngItemModel(&s.Tariff400ngItem),
		Tariff400ngItemID:   handlers.FmtUUID(s.Tariff400ngItemID),
		Location:            apimessages.ShipmentLineItemLocation(s.Location),
		Notes:               s.Notes,
		Description:         s.Description,
		Reason:              s.Reason,
		Quantity1:           handlers.FmtInt64(int64(s.Quantity1)),
		Quantity2:           handlers.FmtInt64(int64(s.Quantity2)),
		Status:              apimessages.ShipmentLineItemStatus(s.Status),
		InvoiceID:           handlers.FmtUUIDPtr(s.InvoiceID),
		ItemDimensions:      payloadForDimensionsModel(&s.ItemDimensions),
		CrateDimensions:     payloadForDimensionsModel(&s.CrateDimensions),
		EstimateAmountCents: handlers.FmtCost(s.EstimateAmountCents),
		ActualAmountCents:   handlers.FmtCost(s.ActualAmountCents),
		AmountCents:         handlers.FmtCost(s.AmountCents),
		AppliedRate:         handlers.FmtMilliCentsPtr(s.AppliedRate),
		Date:                handlers.FmtDatePtr(s.Date),
		Time:                s.Time,
		Address:             payloadForAddressModel(&s.Address),
		SubmittedDate:       *handlers.FmtDateTime(s.SubmittedDate),
		ApprovedDate:        handlers.FmtDateTime(s.ApprovedDate),
	}
}

func payloadForDimensionsModel(a *models.ShipmentLineItemDimensions) *apimessages.Dimensions {
	if a == nil {
		return nil
	}
	if a.ID == uuid.Nil {
		return nil
	}

	return &apimessages.Dimensions{
		ID:     *handlers.FmtUUID(a.ID),
		Length: handlers.FmtInt64(int64(a.Length)),
		Width:  handlers.FmtInt64(int64(a.Width)),
		Height: handlers.FmtInt64(int64(a.Height)),
	}
}

// GetShipmentLineItemsHandler returns a particular shipment line item
type GetShipmentLineItemsHandler struct {
	handlers.HandlerContext
	shipmentLineItemFetcher services.ShipmentLineItemFetcher
}

// Handle returns a specified shipment line item
func (h GetShipmentLineItemsHandler) Handle(params accessorialop.GetShipmentLineItemsParams) middleware.Responder {

	session, logger := h.SessionAndLoggerFromRequest(params.HTTPRequest)
	shipmentID := uuid.Must(uuid.FromString(params.ShipmentID.String()))

	shipmentLineItems, err := h.shipmentLineItemFetcher.GetShipmentLineItemsByShipmentID(shipmentID, session)
	if err != nil {
		logger.Error(fmt.Sprintf("Error fetching line items for shipment %s", shipmentID),
			zap.Error(err))
		return handlers.ResponseForError(logger, err)
	}

	payload := payloadForShipmentLineItemModels(shipmentLineItems)
	return accessorialop.NewGetShipmentLineItemsOK().WithPayload(payload)
}

// CreateShipmentLineItemHandler creates a shipment_line_item for a provided shipment_id
type CreateShipmentLineItemHandler struct {
	handlers.HandlerContext
}

// Handle handles the request
func (h CreateShipmentLineItemHandler) Handle(params accessorialop.CreateShipmentLineItemParams) middleware.Responder {
	session, logger := h.SessionAndLoggerFromRequest(params.HTTPRequest)

	shipmentID := uuid.Must(uuid.FromString(params.ShipmentID.String()))
	var shipment *models.Shipment
	var err error
	// If TSP user, verify TSP has shipment
	// If office user, no verification necessary
	// If myApp user, user is forbidden
	if session.IsTspUser() {
		// Check that the TSP user can access the shipment
		_, shipment, err = models.FetchShipmentForVerifiedTSPUser(h.DB(), session.TspUserID, shipmentID)
		if err != nil {
			logger.Error("Error fetching shipment for TSP user", zap.Error(err))
			return handlers.ResponseForError(logger, err)
		}
	} else if session.IsOfficeUser() {
		shipment, err = models.FetchShipment(h.DB(), session, shipmentID)
		if err != nil {
			logger.Error("Error fetching shipment for office user", zap.Error(err))
			return handlers.ResponseForError(logger, err)
		}
	} else {
		return accessorialop.NewCreateShipmentLineItemForbidden()
	}

	tariff400ngItemID := uuid.Must(uuid.FromString(params.Payload.Tariff400ngItemID.String()))
	tariff400ngItem, err := models.FetchTariff400ngItem(h.DB(), tariff400ngItemID)
	if err != nil {
		return handlers.ResponseForError(logger, err)
	}

	if !tariff400ngItem.RequiresPreApproval {
		return accessorialop.NewCreateShipmentLineItemForbidden()
	}

	baseParams := models.BaseShipmentLineItemParams{
		Tariff400ngItemID:   tariff400ngItemID,
		Tariff400ngItemCode: tariff400ngItem.Code,
		Quantity1:           unit.IntToBaseQuantity(params.Payload.Quantity1),
		Quantity2:           unit.IntToBaseQuantity(params.Payload.Quantity2),
		Location:            string(params.Payload.Location),
		Notes:               handlers.FmtString(params.Payload.Notes),
	}

	var itemDimensions, crateDimensions *models.AdditionalLineItemDimensions
	if params.Payload.ItemDimensions != nil {
		itemDimensions = &models.AdditionalLineItemDimensions{
			Length: unit.ThousandthInches(*params.Payload.ItemDimensions.Length),
			Width:  unit.ThousandthInches(*params.Payload.ItemDimensions.Width),
			Height: unit.ThousandthInches(*params.Payload.ItemDimensions.Height),
		}
	}
	if params.Payload.CrateDimensions != nil {
		crateDimensions = &models.AdditionalLineItemDimensions{
			Length: unit.ThousandthInches(*params.Payload.CrateDimensions.Length),
			Width:  unit.ThousandthInches(*params.Payload.CrateDimensions.Width),
			Height: unit.ThousandthInches(*params.Payload.CrateDimensions.Height),
		}
	}

	additionalParams := models.AdditionalShipmentLineItemParams{
		ItemDimensions:      itemDimensions,
		CrateDimensions:     crateDimensions,
		Description:         params.Payload.Description,
		Reason:              params.Payload.Reason,
		EstimateAmountCents: handlers.FmtInt64PtrToPopPtr(params.Payload.EstimateAmountCents),
		ActualAmountCents:   handlers.FmtInt64PtrToPopPtr(params.Payload.ActualAmountCents),
		Date:                handlers.FmtDatePtrToPopPtr(params.Payload.Date),
		Time:                params.Payload.Time,
		Address:             addressModelFromPayload(params.Payload.Address),
	}

	shipmentLineItem, verrs, err := shipment.CreateShipmentLineItem(h.DB(),
		baseParams,
		additionalParams,
	)

	if verrs.HasAny() || err != nil {
		logger.Error("Error fetching shipment line items for shipment", zap.Error(err))
		return handlers.ResponseForVErrors(logger, verrs, err)
	}
	payload := payloadForShipmentLineItemModel(shipmentLineItem)
	return accessorialop.NewCreateShipmentLineItemCreated().WithPayload(payload)
}

// UpdateShipmentLineItemHandler updates a particular shipment line item
type UpdateShipmentLineItemHandler struct {
	handlers.HandlerContext
}

// Handle updates a specified shipment line item
func (h UpdateShipmentLineItemHandler) Handle(params accessorialop.UpdateShipmentLineItemParams) middleware.Responder {
	session, logger := h.SessionAndLoggerFromRequest(params.HTTPRequest)
	shipmentLineItemID := uuid.Must(uuid.FromString(params.ShipmentLineItemID.String()))
	var shipment *models.Shipment

	// Fetch shipment line item
	shipmentLineItem, err := models.FetchShipmentLineItemByID(h.DB(), &shipmentLineItemID)
	if err != nil {
		logger.Error("Error fetching shipment line item for shipment", zap.Error(err))
		return accessorialop.NewUpdateShipmentLineItemInternalServerError()
	}

	// authorization
	if session.IsTspUser() {
		// Check that the TSP user can access the shipment
		_, shipment, err = models.FetchShipmentForVerifiedTSPUser(h.DB(), session.TspUserID, shipmentLineItem.ShipmentID)
		if err != nil {
			logger.Error("Error fetching shipment for TSP user", zap.Error(err))
			return handlers.ResponseForError(logger, err)
		}
	} else if session.IsOfficeUser() {
		shipment, err = models.FetchShipment(h.DB(), session, shipmentLineItem.ShipmentID)
		if err != nil {
			logger.Error("Error fetching shipment for office user", zap.Error(err))
			return handlers.ResponseForError(logger, err)
		}
	} else {
		return accessorialop.NewUpdateShipmentLineItemForbidden()
	}
	shipmentLineItem.Shipment = *shipment

	tariff400ngItemID := uuid.Must(uuid.FromString(params.Payload.Tariff400ngItemID.String()))
	tariff400ngItem, err := models.FetchTariff400ngItem(h.DB(), tariff400ngItemID)
	// 35A has special functionality to update ActualAmountCents if it is not invoiced and Status is approved
	canUpdate35A := tariff400ngItem.Code == "35A" && shipmentLineItem.EstimateAmountCents != nil && shipmentLineItem.InvoiceID == nil

	if !tariff400ngItem.RequiresPreApproval {
		logger.Error("Error: tariff400ng item " + tariff400ngItem.Code + " does not require pre-approval")
		return accessorialop.NewUpdateShipmentLineItemForbidden()
	} else if shipmentLineItem.Status == models.ShipmentLineItemStatusAPPROVED && !canUpdate35A {
		logger.Error("Error: cannot update shipment line item if status is approved (or status is invoiced for tariff400ng item 35A)")
		return accessorialop.NewUpdateShipmentLineItemUnprocessableEntity()
	}

	baseParams := models.BaseShipmentLineItemParams{
		Tariff400ngItemID:   tariff400ngItemID,
		Tariff400ngItemCode: tariff400ngItem.Code,
		Quantity1:           unit.IntToBaseQuantity(params.Payload.Quantity1),
		Quantity2:           unit.IntToBaseQuantity(params.Payload.Quantity2),
		Location:            string(params.Payload.Location),
		Notes:               handlers.FmtString(params.Payload.Notes),
	}

	var itemDimensions, crateDimensions *models.AdditionalLineItemDimensions
	if params.Payload.ItemDimensions != nil {
		itemDimensions = &models.AdditionalLineItemDimensions{
			Length: unit.ThousandthInches(*params.Payload.ItemDimensions.Length),
			Width:  unit.ThousandthInches(*params.Payload.ItemDimensions.Width),
			Height: unit.ThousandthInches(*params.Payload.ItemDimensions.Height),
		}
	}
	if params.Payload.CrateDimensions != nil {
		crateDimensions = &models.AdditionalLineItemDimensions{
			Length: unit.ThousandthInches(*params.Payload.CrateDimensions.Length),
			Width:  unit.ThousandthInches(*params.Payload.CrateDimensions.Width),
			Height: unit.ThousandthInches(*params.Payload.CrateDimensions.Height),
		}
	}

	additionalParams := models.AdditionalShipmentLineItemParams{
		ItemDimensions:      itemDimensions,
		CrateDimensions:     crateDimensions,
		Description:         params.Payload.Description,
		Reason:              params.Payload.Reason,
		EstimateAmountCents: handlers.FmtInt64PtrToPopPtr(params.Payload.EstimateAmountCents),
		ActualAmountCents:   handlers.FmtInt64PtrToPopPtr(params.Payload.ActualAmountCents),
		Date:                handlers.FmtDatePtrToPopPtr(params.Payload.Date),
		Time:                params.Payload.Time,
		Address:             addressModelFromPayload(params.Payload.Address),
	}

	verrs, err := shipment.UpdateShipmentLineItem(h.DB(),
		baseParams,
		additionalParams,
		&shipmentLineItem,
	)
	if verrs.HasAny() || err != nil {
		logger.Error("Error fetching shipment line items for shipment", zap.Error(err))
		return handlers.ResponseForVErrors(logger, verrs, err)
	}

	if (shipmentLineItem.Status == models.ShipmentLineItemStatusCONDITIONALLYAPPROVED || shipmentLineItem.Status == models.ShipmentLineItemStatusAPPROVED) && shipmentLineItem.ActualAmountCents != nil {
		// If shipment is delivered, price single shipment line item
		if shipmentLineItem.Shipment.Status == models.ShipmentStatusDELIVERED {
			engine := rateengine.NewRateEngine(h.DB(), logger)
			err = engine.PriceAdditionalRequest(&shipmentLineItem)
			if err != nil {
				return handlers.ResponseForError(logger, err)
			}
		}

		// Approve the shipment line item
		if shipmentLineItem.Status == models.ShipmentLineItemStatusCONDITIONALLYAPPROVED {
			err = shipmentLineItem.Approve()
			if err != nil {
				logger.Error("Error approving shipment line item for shipment", zap.Error(err))
				return accessorialop.NewApproveShipmentLineItemForbidden()
			}
		}
	}

	if (shipmentLineItem.Status == models.ShipmentLineItemStatusCONDITIONALLYAPPROVED || shipmentLineItem.Status == models.ShipmentLineItemStatusAPPROVED) && shipmentLineItem.ActualAmountCents == nil {
		if shipmentLineItem.Shipment.Status == models.ShipmentStatusDELIVERED {
			// Unprice request
			shipmentLineItem.AmountCents = nil
			shipmentLineItem.AppliedRate = nil
		}

		if shipmentLineItem.Status == models.ShipmentLineItemStatusAPPROVED {
			// Conditionally approve the shipment line item
			err = shipmentLineItem.ConditionallyApprove()
			if err != nil {
				logger.Error("Error conditionally approving shipment line item for shipment", zap.Error(err))
				return accessorialop.NewApproveShipmentLineItemForbidden()
			}
		}
	}

	h.DB().ValidateAndUpdate(&shipmentLineItem)

	payload := payloadForShipmentLineItemModel(&shipmentLineItem)
	return accessorialop.NewUpdateShipmentLineItemOK().WithPayload(payload)
}

// DeleteShipmentLineItemHandler deletes a particular shipment line item
type DeleteShipmentLineItemHandler struct {
	handlers.HandlerContext
}

// Handle deletes a specified shipment line item
func (h DeleteShipmentLineItemHandler) Handle(params accessorialop.DeleteShipmentLineItemParams) middleware.Responder {
	session, logger := h.SessionAndLoggerFromRequest(params.HTTPRequest)

	// Fetch shipment line item first
	shipmentLineItemID := uuid.Must(uuid.FromString(params.ShipmentLineItemID.String()))
	shipmentLineItem, err := models.FetchShipmentLineItemByID(h.DB(), &shipmentLineItemID)
	if err != nil {
		if errors.Cause(err) == sql.ErrNoRows {
			logger.Error("Error shipment line item for shipment not found", zap.Error(err))
			return accessorialop.NewDeleteShipmentLineItemNotFound()
		}

		logger.Error("Error fetching shipment line item for shipment", zap.Error(err))
		return accessorialop.NewDeleteShipmentLineItemInternalServerError()
	}

	if !shipmentLineItem.Tariff400ngItem.RequiresPreApproval {
		return accessorialop.NewDeleteShipmentLineItemForbidden()
	}

	// authorization
	shipmentID := uuid.Must(uuid.FromString(shipmentLineItem.ShipmentID.String()))
	if session.IsTspUser() {
		// Check that the TSP user can access the shipment
		_, _, fetchShipmentForVerifiedTSPUserErr := models.FetchShipmentForVerifiedTSPUser(h.DB(), session.TspUserID, shipmentID)
		if fetchShipmentForVerifiedTSPUserErr != nil {
			logger.Error("Error fetching shipment for TSP user", zap.Error(fetchShipmentForVerifiedTSPUserErr))
			return handlers.ResponseForError(logger, fetchShipmentForVerifiedTSPUserErr)
		}
	} else if session.IsOfficeUser() {
		_, fetchShipmentErr := models.FetchShipment(h.DB(), session, shipmentID)
		if fetchShipmentErr != nil {
			logger.Error("Error fetching shipment for office user", zap.Error(fetchShipmentErr))
			return handlers.ResponseForError(logger, fetchShipmentErr)
		}
	} else {
		return accessorialop.NewDeleteShipmentLineItemForbidden()
	}

	// Delete the shipment line item
	err = h.DB().Destroy(&shipmentLineItem)
	if err != nil {
		logger.Error("Error deleting shipment line item for shipment", zap.Error(err))
		return handlers.ResponseForError(logger, err)
	}

	payload := payloadForShipmentLineItemModel(&shipmentLineItem)
	return accessorialop.NewDeleteShipmentLineItemOK().WithPayload(payload)
}

// ApproveShipmentLineItemHandler returns a particular shipment
type ApproveShipmentLineItemHandler struct {
	handlers.HandlerContext
}

// Handle returns a specified shipment
func (h ApproveShipmentLineItemHandler) Handle(params accessorialop.ApproveShipmentLineItemParams) middleware.Responder {
	session, logger := h.SessionAndLoggerFromRequest(params.HTTPRequest)
	var shipment *models.Shipment

	shipmentLineItemID := uuid.Must(uuid.FromString(params.ShipmentLineItemID.String()))

	shipmentLineItem, err := models.FetchShipmentLineItemByID(h.DB(), &shipmentLineItemID)
	if err != nil {
		logger.Error("Error fetching line items for shipment", zap.Error(err))
		return accessorialop.NewApproveShipmentLineItemInternalServerError()
	}

	// Non-accessorial line items shouldn't require approval
	// Only office users can approve a shipment line item
	if shipmentLineItem.Tariff400ngItem.RequiresPreApproval && session.IsOfficeUser() {
		shipment, err = models.FetchShipment(h.DB(), session, shipmentLineItem.ShipmentID)
		if err != nil {
			logger.Error("Error fetching shipment for office user", zap.Error(err))
			return handlers.ResponseForError(logger, err)
		}
	} else {
		logger.Error("Error does not require pre-approval for shipment")
		return accessorialop.NewApproveShipmentLineItemForbidden()
	}
	shipmentLineItem.Shipment = *shipment

	if shipmentLineItem.Tariff400ngItem.Code == "35A" && shipmentLineItem.EstimateAmountCents != nil && shipmentLineItem.ActualAmountCents == nil {
		// Conditionally approve the shipment line item
		err = shipmentLineItem.ConditionallyApprove()
		if err != nil {
			logger.Error("Error conditionally approving shipment line item for shipment", zap.Error(err))
			return accessorialop.NewApproveShipmentLineItemForbidden()
		}
	} else {
		// Approve the shipment line item
		err = shipmentLineItem.Approve()
		if err != nil {
			logger.Error("Error approving shipment line item for shipment",
				zap.String("item code", shipmentLineItem.Tariff400ngItem.Code),
				zap.Error(err))
			return accessorialop.NewApproveShipmentLineItemForbidden()
		}
	}

	// If shipment is delivered and line item is approved, price single shipment line item
	if shipmentLineItem.Shipment.Status == models.ShipmentStatusDELIVERED && shipmentLineItem.Status == models.ShipmentLineItemStatusAPPROVED {
		engine := rateengine.NewRateEngine(h.DB(), logger)
		err = engine.PriceAdditionalRequest(&shipmentLineItem)
		if err != nil {
			return handlers.ResponseForError(logger, err)
		}
	}

	// Save the shipment line item
	h.DB().ValidateAndUpdate(&shipmentLineItem)

	payload := payloadForShipmentLineItemModel(&shipmentLineItem)
	return accessorialop.NewApproveShipmentLineItemOK().WithPayload(payload)
}

// RecalculateShipmentLineItemsHandler recalculates shipment line items for a given shipment id
type RecalculateShipmentLineItemsHandler struct {
	handlers.HandlerContext
	shipmentLineItemRecalculator services.ShipmentLineItemRecalculator
}

// Handle handles the recalculation of shipment line items using the appropriate service object
func (h RecalculateShipmentLineItemsHandler) Handle(params accessorialop.RecalculateShipmentLineItemsParams) middleware.Responder {
	session, logger := h.SessionAndLoggerFromRequest(params.HTTPRequest)

	shipmentID := uuid.Must(uuid.FromString(params.ShipmentID.String()))

	shipmentLineItems, err := h.shipmentLineItemRecalculator.RecalculateShipmentLineItems(shipmentID, session, h.Planner())

	if err != nil {
		logger.Error(fmt.Sprintf("Error recalculating shipment line for shipment id: %s", shipmentID), zap.Error(err))
		if err != models.ErrFetchForbidden {
			err = errors.New(fmt.Sprintf("User was authorized but failed to recalculate shipment for id %s", shipmentID))
		}
		return handlers.ResponseForError(logger, err)
	}
	payload := payloadForShipmentLineItemModels(shipmentLineItems)
	return accessorialop.NewRecalculateShipmentLineItemsOK().WithPayload(payload)
}
