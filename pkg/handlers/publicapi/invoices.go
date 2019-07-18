package publicapi

import (
	"github.com/go-openapi/runtime/middleware"
	"github.com/gofrs/uuid"
	"go.uber.org/zap"

	"github.com/transcom/mymove/pkg/gen/apimessages"
	accessorialop "github.com/transcom/mymove/pkg/gen/restapi/apioperations/accessorials"
	"github.com/transcom/mymove/pkg/handlers"
	"github.com/transcom/mymove/pkg/models"
)

func payloadForInvoiceModels(a []models.Invoice) apimessages.Invoices {
	payloads := make(apimessages.Invoices, len(a))

	for i, acc := range a {
		payloads[i] = payloadForInvoiceModel(&acc)
	}

	return payloads
}

func payloadForInvoiceModel(a *models.Invoice) *apimessages.Invoice {
	if a == nil {
		return nil
	}

	return &apimessages.Invoice{
		ID:                *handlers.FmtUUID(a.ID),
		ShipmentID:        *handlers.FmtUUID(a.ShipmentID),
		InvoiceNumber:     a.InvoiceNumber,
		ApproverFirstName: a.Approver.FirstName,
		ApproverLastName:  a.Approver.LastName,
		Status:            apimessages.InvoiceStatus(a.Status),
		InvoicedDate:      *handlers.FmtDateTime(a.InvoicedDate),
	}
}

func errorResponseForInvoiceModel(err error, logger handlers.Logger) middleware.Responder {
	message := "Error fetching invoice"
	switch err {
	case models.ErrFetchNotFound:
		message = "Invoice not found"
	case models.ErrFetchForbidden:
		message = "User not permitted to access invoice"
	case models.ErrUserUnauthorized:
		message = "User not authorized to access invoice"
	}
	logger.Error(message, zap.Error(err))
	return handlers.ResponseForErrorWithMessage(logger, err, message)
}

// GetInvoiceHandler returns an invoice
type GetInvoiceHandler struct {
	handlers.HandlerContext
}

// Handle returns a specified invoice
func (h GetInvoiceHandler) Handle(params accessorialop.GetInvoiceParams) middleware.Responder {
	session, logger := h.SessionAndLoggerFromRequest(params.HTTPRequest)

	if session == nil {
		return accessorialop.NewGetInvoiceUnauthorized()
	}

	// Fetch invoice
	invoiceID, _ := uuid.FromString(params.InvoiceID.String())
	invoice, err := models.FetchInvoice(h.DB(), session, invoiceID)
	if err != nil {
		return errorResponseForInvoiceModel(err, logger)
	}

	payload := payloadForInvoiceModel(invoice)
	return accessorialop.NewGetInvoiceOK().WithPayload(payload)
}
