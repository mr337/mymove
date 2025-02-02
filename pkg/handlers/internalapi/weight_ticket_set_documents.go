package internalapi

import (
	"reflect"
	"time"

	"github.com/go-openapi/strfmt"

	"github.com/transcom/mymove/pkg/unit"

	"github.com/transcom/mymove/pkg/gen/internalmessages"
	"github.com/transcom/mymove/pkg/storage"

	"github.com/go-openapi/runtime/middleware"
	"github.com/gofrs/uuid"
	"github.com/honeycombio/beeline-go"

	movedocop "github.com/transcom/mymove/pkg/gen/internalapi/internaloperations/move_docs"
	"github.com/transcom/mymove/pkg/handlers"
	"github.com/transcom/mymove/pkg/models"
)

func payloadForWeightTicketSetMoveDocumentModel(storer storage.FileStorer, weightTicketSet models.WeightTicketSetDocument) (*internalmessages.MoveDocumentPayload, error) {

	documentPayload, err := payloadForDocumentModel(storer, weightTicketSet.MoveDocument.Document)
	if err != nil {
		return nil, err
	}

	var ppmID *strfmt.UUID
	if weightTicketSet.MoveDocument.PersonallyProcuredMoveID != nil {
		ppmID = handlers.FmtUUID(*weightTicketSet.MoveDocument.PersonallyProcuredMoveID)
	}
	var emptyWeight *int64
	if weightTicketSet.EmptyWeight != nil {
		ew := int64(*weightTicketSet.EmptyWeight)
		emptyWeight = &ew
	}
	var fullWeight *int64
	if weightTicketSet.FullWeight != nil {
		fw := int64(*weightTicketSet.FullWeight)
		fullWeight = &fw
	}
	var weighTicketDate *strfmt.Date
	if weightTicketSet.WeightTicketDate != nil {
		weighTicketDate = handlers.FmtDate(*weightTicketSet.WeightTicketDate)
	}
	genericMoveDocumentPayload := internalmessages.MoveDocumentPayload{
		ID:                       handlers.FmtUUID(weightTicketSet.MoveDocument.ID),
		MoveID:                   handlers.FmtUUID(weightTicketSet.MoveDocument.MoveID),
		Document:                 documentPayload,
		Title:                    &weightTicketSet.MoveDocument.Title,
		MoveDocumentType:         internalmessages.MoveDocumentType(weightTicketSet.MoveDocument.MoveDocumentType),
		VehicleNickname:          weightTicketSet.VehicleNickname,
		VehicleOptions:           weightTicketSet.VehicleOptions,
		PersonallyProcuredMoveID: ppmID,
		EmptyWeight:              emptyWeight,
		EmptyWeightTicketMissing: handlers.FmtBool(weightTicketSet.EmptyWeightTicketMissing),
		FullWeight:               fullWeight,
		FullWeightTicketMissing:  handlers.FmtBool(weightTicketSet.FullWeightTicketMissing),
		TrailerOwnershipMissing:  handlers.FmtBool(weightTicketSet.TrailerOwnershipMissing),
		WeightTicketDate:         weighTicketDate,
		Status:                   internalmessages.MoveDocumentStatus(weightTicketSet.MoveDocument.Status),
		Notes:                    weightTicketSet.MoveDocument.Notes,
	}

	return &genericMoveDocumentPayload, nil
}

// CreateWeightTicketSetDocumentHandler creates a WeightTicketSetDocument
type CreateWeightTicketSetDocumentHandler struct {
	handlers.HandlerContext
}

// Handle is the handler for CreateWeightTicketSetDocumentHandler
func (h CreateWeightTicketSetDocumentHandler) Handle(params movedocop.CreateWeightTicketDocumentParams) middleware.Responder {

	session, logger := h.SessionAndLoggerFromRequest(params.HTTPRequest)

	ctx, span := beeline.StartSpan(params.HTTPRequest.Context(), reflect.TypeOf(h).Name())
	defer span.Send()

	// #nosec UUID is pattern matched by swagger and will be ok
	moveID, _ := uuid.FromString(params.MoveID.String())

	// Validate that this move belongs to the current user
	move, err := models.FetchMove(h.DB(), session, moveID)
	if err != nil {
		return handlers.ResponseForError(logger, err)
	}

	payload := params.CreateWeightTicketDocument
	uploadIds := payload.UploadIds
	uploads := models.Uploads{}
	for _, id := range uploadIds {
		converted := uuid.Must(uuid.FromString(id.String()))
		upload, fetchUploadErr := models.FetchUpload(ctx, h.DB(), session, converted)
		if fetchUploadErr != nil {
			return handlers.ResponseForError(logger, fetchUploadErr)
		}
		uploads = append(uploads, upload)
	}

	ppmID := uuid.Must(uuid.FromString(payload.PersonallyProcuredMoveID.String()))

	// Enforce that the ppm's move_id matches our move
	ppm, fetchPPMErr := models.FetchPersonallyProcuredMove(h.DB(), session, ppmID)
	if fetchPPMErr != nil {
		return handlers.ResponseForError(logger, fetchPPMErr)
	}
	if ppm.MoveID != moveID {
		return movedocop.NewCreateWeightTicketDocumentBadRequest()
	}
	var emptyWeight *unit.Pound
	if payload.EmptyWeight != nil {
		pound := unit.Pound(*payload.EmptyWeight)
		emptyWeight = &pound
	}
	var fullWeight *unit.Pound
	if payload.FullWeight != nil {
		pound := unit.Pound(*payload.FullWeight)
		fullWeight = &pound
	}
	var weighTicketDate *time.Time
	if payload.WeightTicketDate != nil {
		weighTicketDate = (*time.Time)(payload.WeightTicketDate)
	}

	wtsd := models.WeightTicketSetDocument{
		EmptyWeight:              emptyWeight,
		EmptyWeightTicketMissing: *payload.EmptyWeightTicketMissing,
		FullWeight:               fullWeight,
		FullWeightTicketMissing:  *payload.FullWeightTicketMissing,
		VehicleNickname:          *payload.VehicleNickname,
		VehicleOptions:           *payload.VehicleOptions,
		WeightTicketDate:         weighTicketDate,
		TrailerOwnershipMissing:  *payload.TrailerOwnershipMissing,
	}
	newWeightTicketSetDocument, verrs, err := move.CreateWeightTicketSetDocument(
		h.DB(),
		uploads,
		&ppmID,
		&wtsd,
		*move.SelectedMoveType,
	)

	if err != nil || verrs.HasAny() {
		return handlers.ResponseForVErrors(logger, verrs, err)
	}

	newPayload, err := payloadForWeightTicketSetMoveDocumentModel(h.FileStorer(), *newWeightTicketSetDocument)
	if err != nil {
		return handlers.ResponseForError(logger, err)
	}
	return movedocop.NewCreateWeightTicketDocumentOK().WithPayload(newPayload)
}
