package testdatagen

import (
	"github.com/gobuffalo/pop"

	"github.com/transcom/mymove/pkg/models"
	"github.com/transcom/mymove/pkg/unit"
)

// MakeWeightTicketSetDocument creates a single Weight Ticket Set Document.
func MakeWeightTicketSetDocument(db *pop.Connection, assertions Assertions) models.WeightTicketSetDocument {
	moveDoc := assertions.WeightTicketSetDocument.MoveDocument
	// ID is required because it must be populated for Eager saving to work.
	if isZeroUUID(moveDoc.MoveID) {
		moveDoc = MakeMoveDocument(db, assertions)
	}

	emptyWeight := unit.Pound(1000)
	fullWeight := unit.Pound(2500)
	weightTicketSetDocument := models.WeightTicketSetDocument{
		MoveDocumentID:   moveDoc.ID,
		MoveDocument:     moveDoc,
		EmptyWeight:      &emptyWeight,
		FullWeight:       &fullWeight,
		VehicleNickname:  "My Car",
		VehicleOptions:   "CAR",
		WeightTicketDate: &NextValidMoveDate,
	}

	// Overwrite values with those from assertions
	mergeModels(&weightTicketSetDocument, assertions.WeightTicketSetDocument)

	mustCreate(db, &weightTicketSetDocument)

	return weightTicketSetDocument
}

// MakeDefaultWeightTicketSetDocument returns a MoveDocument with default values
func MakeDefaultWeightTicketSetDocument(db *pop.Connection) models.WeightTicketSetDocument {
	return MakeWeightTicketSetDocument(db, Assertions{})
}
