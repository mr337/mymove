package scenario

import (
	"bytes"
	"fmt"
	"log"
	"time"

	"github.com/go-openapi/swag"
	"github.com/gobuffalo/pop"
	"github.com/gofrs/uuid"

	"github.com/transcom/mymove/pkg/auth"
	"github.com/transcom/mymove/pkg/gen/apimessages"
	"github.com/transcom/mymove/pkg/handlers"
	storageintransit "github.com/transcom/mymove/pkg/services/storage_in_transit"

	"github.com/transcom/mymove/pkg/assets"
	"github.com/transcom/mymove/pkg/dates"
	"github.com/transcom/mymove/pkg/gen/internalmessages"
	"github.com/transcom/mymove/pkg/models"
	"github.com/transcom/mymove/pkg/paperwork"
	"github.com/transcom/mymove/pkg/rateengine"
	"github.com/transcom/mymove/pkg/route"
	shipmentservice "github.com/transcom/mymove/pkg/services/shipment"
	"github.com/transcom/mymove/pkg/storage"
	"github.com/transcom/mymove/pkg/testdatagen"
	"github.com/transcom/mymove/pkg/unit"
	"github.com/transcom/mymove/pkg/uploader"
	uploaderpkg "github.com/transcom/mymove/pkg/uploader"
)

// E2eBasicScenario builds a basic set of data for e2e testing
type e2eBasicScenario NamedScenario

// E2eBasicScenario Is the thing
var E2eBasicScenario = e2eBasicScenario{"e2e_basic"}

var selectedMoveTypeHHG = models.SelectedMoveTypeHHG
var selectedMoveTypeHHGPPM = models.SelectedMoveTypeHHGPPM

// Often weekends and holidays are not allowable dates
var cal = dates.NewUSCalendar()
var nextValidMoveDate = dates.NextValidMoveDate(time.Now(), cal)

var nextValidMoveDatePlusOne = dates.NextValidMoveDate(nextValidMoveDate.AddDate(0, 0, 1), cal)
var nextValidMoveDatePlusFive = dates.NextValidMoveDate(nextValidMoveDate.AddDate(0, 0, 5), cal)
var nextValidMoveDatePlusTen = dates.NextValidMoveDate(nextValidMoveDate.AddDate(0, 0, 10), cal)

var nextValidMoveDateMinusOne = dates.NextValidMoveDate(nextValidMoveDate.AddDate(0, 0, -1), cal)
var nextValidMoveDateMinusFive = dates.NextValidMoveDate(nextValidMoveDate.AddDate(0, 0, -5), cal)
var nextValidMoveDateMinusTen = dates.NextValidMoveDate(nextValidMoveDate.AddDate(0, 0, -10), cal)

// Run does that data load thing
func (e e2eBasicScenario) Run(db *pop.Connection, loader *uploader.Uploader, logger Logger, storer *storage.Memory) {
	/*
	 * Basic user with tsp access
	 */
	email := "tspuser1@example.com"
	tspUser := testdatagen.MakeTspUser(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("6cd03e5b-bee8-4e97-a340-fecb8f3d5465")),
			LoginGovEmail: email,
		},
		TspUser: models.TspUser{
			ID:    uuid.FromStringOrNil("1fb58b82-ab60-4f55-a654-0267200473a4"),
			Email: email,
		},
		TransportationServiceProvider: models.TransportationServiceProvider{
			StandardCarrierAlphaCode: "J12K",
		},
	})
	tspUserSession := auth.Session{
		ApplicationName: auth.TspApp,
		UserID:          *tspUser.UserID,
		IDToken:         "fake token",
		TspUserID:       tspUser.ID,
	}

	/*
	 * Basic user with office access
	 */
	email = "officeuser1@example.com"
	testdatagen.MakeOfficeUser(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("9bfa91d2-7a0c-4de0-ae02-b8cf8b4b858b")),
			LoginGovEmail: email,
		},
		OfficeUser: models.OfficeUser{
			ID:    uuid.FromStringOrNil("9c5911a7-5885-4cf4-abec-021a40692403"),
			Email: email,
		},
	})

	/*
	 * Service member with uploaded orders and an approved shipment to be accepted & able to generate GBL
	 */
	MakeHhgFromAwardedToAcceptedGBLReady(db, tspUser)

	/*
	 * Service member with uploaded orders and an approved shipment to be accepted & GBL generated
	 */
	MakeHhgWithGBL(db, tspUser, logger, storer)

	/*
	 * Service member with an approved shipment and submitted PPM
	 */
	MakeHhgWithPpm(db, tspUser, loader)

	/*
	 * Service member with uploaded orders and a delivered shipment, able to generate GBL
	 */
	makeHhgReadyToInvoice(db, tspUser, logger, storer)

	/*
	 * Service member with uploaded orders and an approved shipment but show in the moves table is false
	 */
	makeHhgShipment(db, tspUser)

	/*
	 * Service member with no uploaded orders
	 */
	email = "needs@orde.rs"
	uuidStr := "feac0e92-66ec-4cab-ad29-538129bf918e"
	testdatagen.MakeUser(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString(uuidStr)),
			LoginGovEmail: email,
		},
	})

	testdatagen.MakeExtendedServiceMember(db, testdatagen.Assertions{
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("c52a9f13-ccc7-4c1b-b5ef-e1132a4f4db9"),
			UserID:        uuid.FromStringOrNil(uuidStr),
			FirstName:     models.StringPointer("NEEDS"),
			LastName:      models.StringPointer("ORDERS"),
			PersonalEmail: models.StringPointer(email),
		},
	})

	/*
	 * Service member with uploaded orders and a new ppm
	 */
	email = "ppm@incomple.te"
	uuidStr = "e10d5964-c070-49cb-9bd1-eaf9f7348eb6"
	testdatagen.MakeUser(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString(uuidStr)),
			LoginGovEmail: email,
		},
	})
	advance := models.BuildDraftReimbursement(1000, models.MethodOfReceiptMILPAY)
	ppm0 := testdatagen.MakePPM(db, testdatagen.Assertions{
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("94ced723-fabc-42af-b9ee-87f8986bb5c9"),
			UserID:        uuid.FromStringOrNil(uuidStr),
			FirstName:     models.StringPointer("PPM"),
			LastName:      models.StringPointer("Submitted"),
			Edipi:         models.StringPointer("1234567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:      uuid.FromStringOrNil("0db80bd6-de75-439e-bf89-deaafa1d0dc8"),
			Locator: "VGHEIS",
		},
		PersonallyProcuredMove: models.PersonallyProcuredMove{
			OriginalMoveDate:    &nextValidMoveDate,
			Advance:             &advance,
			AdvanceID:           &advance.ID,
			HasRequestedAdvance: true,
		},
		Uploader: loader,
	})
	ppm0.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &ppm0.Move)

	/*
	 * Service member with uploaded orders, a new ppm and no advance
	 */
	email = "ppm@advance.no"
	uuidStr = "f0ddc118-3f7e-476b-b8be-0f964a5feee2"
	testdatagen.MakeUser(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString(uuidStr)),
			LoginGovEmail: email,
		},
	})
	ppmNoAdvance := testdatagen.MakePPM(db, testdatagen.Assertions{
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("1a1aafde-df3b-4459-9dbd-27e9f6c1d2f6"),
			UserID:        uuid.FromStringOrNil(uuidStr),
			FirstName:     models.StringPointer("PPM"),
			LastName:      models.StringPointer("No Advance"),
			Edipi:         models.StringPointer("1234567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:      uuid.FromStringOrNil("4f3f4bee-3719-4c17-8cf4-7e445a38d90e"),
			Locator: "NOADVC",
		},
		PersonallyProcuredMove: models.PersonallyProcuredMove{
			OriginalMoveDate: &nextValidMoveDate,
		},
		Uploader: loader,
	})
	ppmNoAdvance.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &ppmNoAdvance.Move)

	/*
	 * office user finds the move: office user completes storage panel
	 */
	email = "office.user.completes@storage.panel"
	uuidStr = "ebac4efd-c980-48d6-9cce-99fb34644789"
	testdatagen.MakeUser(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString(uuidStr)),
			LoginGovEmail: email,
		},
	})
	ppmStorage := testdatagen.MakePPM(db, testdatagen.Assertions{
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("76eb1c93-16f7-4c8e-a71c-67d5c9093dd3"),
			UserID:        uuid.FromStringOrNil(uuidStr),
			FirstName:     models.StringPointer("Storage"),
			LastName:      models.StringPointer("Panel"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:      uuid.FromStringOrNil("25fb9bf6-2a38-4463-8247-fce2a5571ab7"),
			Locator: "STORAG",
		},
		PersonallyProcuredMove: models.PersonallyProcuredMove{
			OriginalMoveDate: &nextValidMoveDate,
		},
		Uploader: loader,
	})
	ppmStorage.Move.Submit(time.Now())
	ppmStorage.Move.Approve()
	ppmStorage.Move.PersonallyProcuredMoves[0].Submit(time.Now())
	ppmStorage.Move.PersonallyProcuredMoves[0].Approve(time.Now())
	ppmStorage.Move.PersonallyProcuredMoves[0].RequestPayment()
	models.SaveMoveDependencies(db, &ppmStorage.Move)

	/*
	 * office user finds the move: office user cancels storage panel
	 */
	email = "office.user.cancelss@storage.panel"
	uuidStr = "cbb56f00-97f7-4d20-83cf-25a7b2f150b6"
	testdatagen.MakeUser(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString(uuidStr)),
			LoginGovEmail: email,
		},
	})
	ppmNoStorage := testdatagen.MakePPM(db, testdatagen.Assertions{
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("b9673e29-ac8d-4945-abc2-36f8eafd6fd8"),
			UserID:        uuid.FromStringOrNil(uuidStr),
			FirstName:     models.StringPointer("Storage"),
			LastName:      models.StringPointer("Panel"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:      uuid.FromStringOrNil("9d0409b8-3587-4fad-9caf-7fc853e1c001"),
			Locator: "NOSTRG",
		},
		PersonallyProcuredMove: models.PersonallyProcuredMove{
			OriginalMoveDate: &nextValidMoveDate,
		},
		Uploader: loader,
	})
	ppmNoStorage.Move.Submit(time.Now())
	ppmNoStorage.Move.Approve()
	ppmNoStorage.Move.PersonallyProcuredMoves[0].Submit(time.Now())
	ppmNoStorage.Move.PersonallyProcuredMoves[0].Approve(time.Now())
	ppmNoStorage.Move.PersonallyProcuredMoves[0].RequestPayment()
	models.SaveMoveDependencies(db, &ppmNoStorage.Move)

	/*
	 * A move, that will be canceled by the E2E test
	 */
	email = "ppm-to-cancel@example.com"
	uuidStr = "e10d5964-c070-49cb-9bd1-eaf9f7348eb7"
	testdatagen.MakeUser(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString(uuidStr)),
			LoginGovEmail: email,
		},
	})
	ppmToCancel := testdatagen.MakePPM(db, testdatagen.Assertions{
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("94ced723-fabc-42af-b9ee-87f8986bb5ca"),
			UserID:        uuid.FromStringOrNil(uuidStr),
			FirstName:     models.StringPointer("PPM"),
			LastName:      models.StringPointer("Submitted"),
			Edipi:         models.StringPointer("1234567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:      uuid.FromStringOrNil("0db80bd6-de75-439e-bf89-deaafa1d0dc9"),
			Locator: "CANCEL",
		},
		PersonallyProcuredMove: models.PersonallyProcuredMove{
			OriginalMoveDate: &nextValidMoveDate,
		},
		Uploader: loader,
	})
	ppmToCancel.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &ppmToCancel.Move)

	/*
	 * Service member with a ppm in progress
	 */
	email = "ppm.on@progre.ss"
	uuidStr = "20199d12-5165-4980-9ca7-19b5dc9f1032"
	testdatagen.MakeUser(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString(uuidStr)),
			LoginGovEmail: email,
		},
	})
	pastTime := nextValidMoveDateMinusTen
	ppm1 := testdatagen.MakePPM(db, testdatagen.Assertions{
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("466c41b9-50bf-462c-b3cd-1ae33a2dad9b"),
			UserID:        uuid.FromStringOrNil(uuidStr),
			FirstName:     models.StringPointer("PPM"),
			LastName:      models.StringPointer("In Progress"),
			Edipi:         models.StringPointer("1617033988"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:      uuid.FromStringOrNil("c9df71f2-334f-4f0e-b2e7-050ddb22efa1"),
			Locator: "GBXYUI",
		},
		PersonallyProcuredMove: models.PersonallyProcuredMove{
			OriginalMoveDate: &pastTime,
		},
		Uploader: loader,
	})
	ppm1.Move.Submit(time.Now())
	ppm1.Move.Approve()
	models.SaveMoveDependencies(db, &ppm1.Move)

	/*
	 * Service member with a ppm move with payment requested
	 */
	email = "ppm@paymentrequest.ed"
	uuidStr = "1842091b-b9a0-4d4a-ba22-1e2f38f26317"
	testdatagen.MakeUser(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString(uuidStr)),
			LoginGovEmail: email,
		},
	})
	futureTime := nextValidMoveDatePlusTen
	typeDetail := internalmessages.OrdersTypeDetailPCSTDY
	ppm2 := testdatagen.MakePPM(db, testdatagen.Assertions{
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("9ce5a930-2446-48ec-a9c0-17bc65e8522d"),
			UserID:        uuid.FromStringOrNil(uuidStr),
			FirstName:     models.StringPointer("PPMPayment"),
			LastName:      models.StringPointer("Requested"),
			Edipi:         models.StringPointer("7617033988"),
			PersonalEmail: models.StringPointer(email),
		},
		// These values should be populated for an approved move
		Order: models.Order{
			OrdersNumber:        models.StringPointer("12345"),
			OrdersTypeDetail:    &typeDetail,
			DepartmentIndicator: models.StringPointer("AIR_FORCE"),
			TAC:                 models.StringPointer("99"),
		},
		Move: models.Move{
			ID:      uuid.FromStringOrNil("0a2580ef-180a-44b2-a40b-291fa9cc13cc"),
			Locator: "FDXTIU",
		},
		PersonallyProcuredMove: models.PersonallyProcuredMove{
			OriginalMoveDate: &futureTime,
		},
		Uploader: loader,
	})
	ppm2.Move.Submit(time.Now())
	ppm2.Move.Approve()
	// This is the same PPM model as ppm2, but this is the one that will be saved by SaveMoveDependencies
	ppm2.Move.PersonallyProcuredMoves[0].Submit(time.Now())
	ppm2.Move.PersonallyProcuredMoves[0].Approve(time.Now())
	ppm2.Move.PersonallyProcuredMoves[0].RequestPayment()
	models.SaveMoveDependencies(db, &ppm2.Move)

	/*
	 * Service member with a ppm move that has requested payment
	 */
	email = "ppmpayment@request.ed"
	uuidStr = "beccca28-6e15-40cc-8692-261cae0d4b14"
	testdatagen.MakeUser(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString(uuidStr)),
			LoginGovEmail: email,
		},
	})
	// Date picked essentialy at random, but needs to be within TestYear
	originalMoveDate := time.Date(testdatagen.TestYear, time.November, 10, 23, 0, 0, 0, time.UTC)
	actualMoveDate := time.Date(testdatagen.TestYear, time.November, 11, 10, 0, 0, 0, time.UTC)
	moveTypeDetail := internalmessages.OrdersTypeDetailPCSTDY
	ppm3 := testdatagen.MakePPM(db, testdatagen.Assertions{
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("3c24bab5-fd13-4057-a321-befb97d90c43"),
			UserID:        uuid.FromStringOrNil(uuidStr),
			FirstName:     models.StringPointer("PPM"),
			LastName:      models.StringPointer("Payment Requested"),
			Edipi:         models.StringPointer("7617033988"),
			PersonalEmail: models.StringPointer(email),
		},
		// These values should be populated for an approved move
		Order: models.Order{
			OrdersNumber:        models.StringPointer("12345"),
			OrdersTypeDetail:    &moveTypeDetail,
			DepartmentIndicator: models.StringPointer("AIR_FORCE"),
			TAC:                 models.StringPointer("99"),
		},
		Move: models.Move{
			ID:      uuid.FromStringOrNil("d6b8980d-6f88-41be-9ae2-1abcbd2574bc"),
			Locator: "PAYMNT",
		},
		PersonallyProcuredMove: models.PersonallyProcuredMove{
			OriginalMoveDate: &originalMoveDate,
			ActualMoveDate:   &actualMoveDate,
		},
		Uploader: loader,
	})
	ppm3.Move.Submit(time.Now())
	ppm3.Move.Approve()
	// This is the same PPM model as ppm3, but this is the one that will be saved by SaveMoveDependencies
	ppm3.Move.PersonallyProcuredMoves[0].Submit(time.Now())
	ppm3.Move.PersonallyProcuredMoves[0].Approve(time.Now())
	ppm3.Move.PersonallyProcuredMoves[0].RequestPayment()
	models.SaveMoveDependencies(db, &ppm3.Move)

	/*
	 * A PPM move that has been canceled.
	 */
	email = "ppm-canceled@example.com"
	uuidStr = "20102768-4d45-449c-a585-81bc386204b1"
	testdatagen.MakeUser(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString(uuidStr)),
			LoginGovEmail: email,
		},
	})
	ppmCanceled := testdatagen.MakePPM(db, testdatagen.Assertions{
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("2da0d5e6-4efb-4ea1-9443-bf9ef64ace65"),
			UserID:        uuid.FromStringOrNil(uuidStr),
			FirstName:     models.StringPointer("PPM"),
			LastName:      models.StringPointer("Canceled"),
			Edipi:         models.StringPointer("1234567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:      uuid.FromStringOrNil("6b88c856-5f41-427e-a480-a7fb6c87533b"),
			Locator: "PPMCAN",
		},
		PersonallyProcuredMove: models.PersonallyProcuredMove{
			OriginalMoveDate: &nextValidMoveDate,
		},
		Uploader: loader,
	})
	ppmCanceled.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &ppmCanceled.Move)
	ppmCanceled.Move.Cancel("reasons")
	models.SaveMoveDependencies(db, &ppmCanceled.Move)

	/*
	 * Service member with orders and a move
	 */
	email = "profile@comple.te"
	uuidStr = "13f3949d-0d53-4be4-b1b1-ae4314793f34"
	testdatagen.MakeUser(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString(uuidStr)),
			LoginGovEmail: email,
		},
	})

	testdatagen.MakeMove(db, testdatagen.Assertions{
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("0a1e72b0-1b9f-442b-a6d3-7b7cfa6bbb95"),
			UserID:        uuid.FromStringOrNil(uuidStr),
			FirstName:     models.StringPointer("Profile"),
			LastName:      models.StringPointer("Complete"),
			Edipi:         models.StringPointer("8893308161"),
			PersonalEmail: models.StringPointer(email),
		},
		Order: models.Order{
			HasDependents:    true,
			SpouseHasProGear: true,
		},
		Move: models.Move{
			ID:      uuid.FromStringOrNil("173da49c-fcec-4d01-a622-3651e81c654e"),
			Locator: "BLABLA",
		},
		Uploader: loader,
	})

	/*
	 * Service member with orders and a move, but no move type selected to select HHG
	 */
	email = "sm_hhg@example.com"
	uuidStr = "4b389406-9258-4695-a091-0bf97b5a132f"

	testdatagen.MakeUser(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString(uuidStr)),
			LoginGovEmail: email,
		},
	})

	testdatagen.MakeMoveWithoutMoveType(db, testdatagen.Assertions{
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("b5d1f44b-5ceb-4a0e-9119-5687808996ff"),
			UserID:        uuid.FromStringOrNil(uuidStr),
			FirstName:     models.StringPointer("HHGDude"),
			LastName:      models.StringPointer("UserPerson"),
			Edipi:         models.StringPointer("6833908163"),
			PersonalEmail: models.StringPointer(email),
		},
		Order: models.Order{
			HasDependents:    true,
			SpouseHasProGear: true,
		},
		Move: models.Move{
			ID:      uuid.FromStringOrNil("8718c8ac-e0c6-423b-bdc6-af971ee05b9a"),
			Locator: "REWGIE",
		},
	})

	/*
	 * Service member with orders and a move, but no move type selected to select HHG
	 */
	email = "sm_hhg_continue@example.com"
	uuidStr = "1256a6ea-27cc-4d60-92df-1bc2a5c39028"

	testdatagen.MakeUser(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString(uuidStr)),
			LoginGovEmail: email,
		},
	})

	testdatagen.MakeMoveWithoutMoveType(db, testdatagen.Assertions{
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("2dac8695-b4d8-40ad-aa84-2b03b6bec960"),
			UserID:        uuid.FromStringOrNil(uuidStr),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("Continue"),
			Edipi:         models.StringPointer("2553282188"),
			PersonalEmail: models.StringPointer(email),
		},
		Order: models.Order{
			HasDependents:    true,
			SpouseHasProGear: true,
		},
		Move: models.Move{
			ID:      uuid.FromStringOrNil("209ecba8-f99d-446f-b649-06a0da613aa7"),
			Locator: "HHGCON",
		},
	})

	/*
	 * Another service member with orders and a move, but no move type selected
	 */
	email = "sm_no_move_type@example.com"
	uuidStr = "9ceb8321-6a82-4f6d-8bb3-a1d85922a202"

	testdatagen.MakeUser(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString(uuidStr)),
			LoginGovEmail: email,
		},
	})

	testdatagen.MakeMoveWithoutMoveType(db, testdatagen.Assertions{
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("7554e347-2215-484f-9240-c61bae050220"),
			UserID:        uuid.FromStringOrNil(uuidStr),
			FirstName:     models.StringPointer("HHGDude2"),
			LastName:      models.StringPointer("UserPerson2"),
			Edipi:         models.StringPointer("6833908164"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:      uuid.FromStringOrNil("b2ecbbe5-36ad-49fc-86c8-66e55e0697a7"),
			Locator: "ZPGVED",
		},
	})

	/*
	 * Service member with uploaded orders and a new shipment move
	 */
	email = "hhg@incomple.te"

	hhg0, err := testdatagen.MakeShipmentForPricing(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("ebc176e0-bb34-47d4-ba37-ff13e2dd40b9")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("0d719b18-81d6-474a-86aa-b87246fff65c"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("Submitted"),
			Edipi:         models.StringPointer("4444567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("2ed0b5a2-26d9-49a3-a775-5220055e8ffe"),
			Locator:          "RLKBEM",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("0dfdbdda-c57e-4b29-994a-09fb8641fc75"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
	})
	if err != nil {
		log.Panic(err)
	}

	hhg0.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg0.Move)

	/*
	 * Service member with uploaded orders and an approved shipment
	 */
	email = "hhg@award.ed"

	offer1 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("7980f0cf-63e3-4722-b5aa-ba46f8f7ac64")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("8a66beef-1cdf-4117-9db2-aad548f54430"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("Submitted"),
			Edipi:         models.StringPointer("4444567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Order: models.Order{
			HasDependents:    true,
			SpouseHasProGear: true,
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("56b8ef45-8145-487b-9b59-0e30d0d465fa"),
			Locator:          "BACON1",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("776b5a23-2830-4de0-bb6a-7698a25865cb"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:             models.ShipmentStatusAWARDED,
			HasDeliveryAddress: true,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
		},
	})

	hhg1 := offer1.Shipment
	hhg1.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg1.Move)

	/*
	 * Service member with uploaded orders and an approved shipment to be accepted
	 */
	email = "hhg@fromawardedtoaccept.ed"

	sourceOffice := testdatagen.MakeTransportationOffice(db, testdatagen.Assertions{
		TransportationOffice: models.TransportationOffice{
			Gbloc: "ABCD",
		},
	})
	destOffice := testdatagen.MakeTransportationOffice(db, testdatagen.Assertions{
		TransportationOffice: models.TransportationOffice{
			Gbloc: "QRED",
		},
	})
	offer2 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("179598c5-a5ee-4da5-8259-29749f03a398")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("179598c5-a5ee-4da5-8259-29749f03a398"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("ReadyForAccept"),
			Edipi:         models.StringPointer("4444567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Order: models.Order{
			DepartmentIndicator: models.StringPointer("17"),
			TAC:                 models.StringPointer("NTA4"),
			SAC:                 models.StringPointer("1234567890 9876543210"),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("849a7880-4a82-4f76-acb4-63cf481e786b"),
			Locator:          "BACON2",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("5f86c201-1abf-4f9d-8dcb-d039cb1c6bfc"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			ID:                          uuid.FromStringOrNil("53ebebef-be58-41ce-9635-a4930149190d"),
			Status:                      models.ShipmentStatusAWARDED,
			PmSurveyPlannedPackDate:     &nextValidMoveDatePlusOne,
			PmSurveyConductedDate:       &nextValidMoveDatePlusOne,
			PmSurveyCompletedAt:         &nextValidMoveDatePlusOne,
			PmSurveyPlannedPickupDate:   &nextValidMoveDatePlusFive,
			PmSurveyPlannedDeliveryDate: &nextValidMoveDatePlusTen,
			SourceGBLOC:                 &sourceOffice.Gbloc,
			DestinationGBLOC:            &destOffice.Gbloc,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			TransportationServiceProvider:   tspUser.TransportationServiceProvider,
		},
	})

	hhg2 := offer2.Shipment
	hhg2.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg2.Move)

	/*
	 * Service member with accepted shipment
	 */
	email = "hhg@accept.ed"

	offer3 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("6a39dd2a-a23f-4967-a035-3bc9987c6848")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("6a39dd2a-a23f-4967-a035-3bc9987c6848"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("ReadyForApprove"),
			Edipi:         models.StringPointer("4444567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("4752270d-4a6f-44ea-82f6-ae3cf3277c5d"),
			Locator:          "BACON3",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("e09f8b8b-67a6-4ce3-b5c3-bd48c82512fc"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:                      models.ShipmentStatusACCEPTED,
			HasDeliveryAddress:          true,
			BookDate:                    &nextValidMoveDate,
			PmSurveyPlannedPackDate:     &nextValidMoveDatePlusOne,
			PmSurveyConductedDate:       &nextValidMoveDatePlusOne,
			PmSurveyCompletedAt:         &nextValidMoveDatePlusOne,
			PmSurveyPlannedPickupDate:   &nextValidMoveDatePlusFive,
			PmSurveyPlannedDeliveryDate: &nextValidMoveDatePlusTen,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			Accepted:                        models.BoolPointer(true),
		},
	})

	hhg3 := offer3.Shipment
	hhg3.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg3.Move)

	/*
	 * Service member with uploaded orders and an approved shipment to have weight added
	 */
	email = "hhg@addweigh.ts"

	offer4 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("bf022aeb-3f14-4429-94d7-fe759f493aed")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("01fa956f-d17b-477e-8607-1db1dd891720"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("Submitted"),
			Edipi:         models.StringPointer("4444567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("94739ee0-664c-47c5-afe9-0f5067a2e151"),
			Locator:          "BACON4",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("9ebc891b-f629-4ea1-9ebf-eef1971d69a3"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:                      models.ShipmentStatusAWARDED,
			BookDate:                    &nextValidMoveDate,
			PmSurveyPlannedPackDate:     &nextValidMoveDatePlusOne,
			PmSurveyConductedDate:       &nextValidMoveDatePlusOne,
			PmSurveyCompletedAt:         &nextValidMoveDatePlusOne,
			PmSurveyPlannedPickupDate:   &nextValidMoveDatePlusFive,
			PmSurveyPlannedDeliveryDate: &nextValidMoveDatePlusTen,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
		},
	})

	hhg4 := offer4.Shipment
	hhg4.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg4.Move)

	/*
	 * Service member with uploaded orders and an approved shipment to have weight added
	 * This shipment is rejected by the e2e test.
	 */
	email = "hhg@reject.ing"
	offer5 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("76bdcff3-ade4-41ff-bf09-0b2474cec751")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("f4e362e9-9fdd-490b-a2fa-1fa4035b8f0d"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("Submitted"),
			Edipi:         models.StringPointer("4444567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("7fca3fd0-08a6-480a-8a9c-16a65a100db9"),
			Locator:          "REJECT",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("1731c3e6-b510-43d0-be46-13e5a2032bad"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:                      models.ShipmentStatusAWARDED,
			BookDate:                    &nextValidMoveDate,
			PmSurveyPlannedPackDate:     &nextValidMoveDatePlusOne,
			PmSurveyConductedDate:       &nextValidMoveDatePlusOne,
			PmSurveyCompletedAt:         &nextValidMoveDatePlusOne,
			PmSurveyPlannedPickupDate:   &nextValidMoveDatePlusFive,
			PmSurveyPlannedDeliveryDate: &nextValidMoveDatePlusTen,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
		},
	})

	hhg5 := offer5.Shipment
	hhg5.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg5.Move)

	/*
	 * Service member with in transit shipment
	 */
	email = "hhg@in.transit"

	offer6 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("1239dd2a-a23f-4967-a035-3bc9987c6848")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("2339dd2a-a23f-4967-a035-3bc9987c6824"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("ReadyForApprove"),
			Edipi:         models.StringPointer("4444567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("3452270d-4a6f-44ea-82f6-ae3cf3277c5d"),
			Locator:          "NINOPK",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("459f8b8b-67a6-4ce3-b5c3-bd48c82512fc"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:                      models.ShipmentStatusINTRANSIT,
			BookDate:                    &nextValidMoveDateMinusTen,
			PmSurveyPlannedPackDate:     &nextValidMoveDateMinusFive,
			PmSurveyConductedDate:       &nextValidMoveDateMinusFive,
			PmSurveyPlannedPickupDate:   &nextValidMoveDateMinusOne,
			PmSurveyPlannedDeliveryDate: &nextValidMoveDatePlusFive,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			Accepted:                        models.BoolPointer(true),
		},
	})

	hhg6 := offer6.Shipment
	hhg6.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg6.Move)

	/*
	 * Service member with approved shipment
	 */
	email = "hhg@approv.ed"

	offer7 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("68461d67-5385-4780-9cb6-417075343b0e")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("2825cadf-410f-4f82-aa0f-4caaf000e63e"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("ReadyForApprove"),
			Edipi:         models.StringPointer("4444567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("616560f2-7e35-4504-b7e6-69038fb0c015"),
			Locator:          "APPRVD",
			SelectedMoveType: &selectedMoveTypeHHG,
			Status:           models.MoveStatusAPPROVED,
		},
		Order: models.Order{
			OrdersNumber:        models.StringPointer("54321"),
			OrdersTypeDetail:    &typeDetail,
			DepartmentIndicator: models.StringPointer("AIR_FORCE"),
			TAC:                 models.StringPointer("99"),
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("5fe59be4-45d0-47c7-b426-cf4db9882af7"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:   models.ShipmentStatusAPPROVED,
			BookDate: &nextValidMoveDate,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			Accepted:                        models.BoolPointer(true),
		},
	})

	hhg7 := offer7.Shipment
	hhg7.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg7.Move)

	/*
	 * Service member with approved basics and accepted shipment
	 */
	email = "hhg@accept.ed"

	offer8 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("f79fd68e-4461-4ba8-b630-9618b913e229")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("f79fd68e-4461-4ba8-b630-9618b913e229"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("ReadyForApprove"),
			Edipi:         models.StringPointer("4444567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("29cd6b2f-9ef2-48be-b4ee-1c1e0a1456ef"),
			Locator:          "BACON5",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		Order: models.Order{
			OrdersNumber:        models.StringPointer("54321"),
			OrdersTypeDetail:    &typeDetail,
			DepartmentIndicator: models.StringPointer("AIR_FORCE"),
			TAC:                 models.StringPointer("99"),
			SAC:                 models.StringPointer("SAC"),
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("d17e2e3e-9bff-4bb0-b301-f97ad03350c1"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status: models.ShipmentStatusACCEPTED,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			Accepted:                        models.BoolPointer(true),
		},
	})

	hhg8 := offer8.Shipment
	hhg8.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg8.Move)

	/*
	 * Service member with uploaded orders, a new shipment move, and a service agent
	 */
	email = "hhg@incomplete.serviceagent"
	hhg9 := testdatagen.MakeShipment(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("412e76e0-bb34-47d4-ba37-ff13e2dd40b9")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("245a9b18-81d6-474a-86aa-b87246fff65c"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("Submitted"),
			Edipi:         models.StringPointer("4444567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("1a3eb5a2-26d9-49a3-a775-5220055e8ffe"),
			Locator:          "LRKREK",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("873dbdda-c57e-4b29-994a-09fb8641fc75"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
	})
	testdatagen.MakeServiceAgent(db, testdatagen.Assertions{
		ServiceAgent: models.ServiceAgent{
			ShipmentID: hhg9.ID,
		},
	})
	testdatagen.MakeServiceAgent(db, testdatagen.Assertions{
		ServiceAgent: models.ServiceAgent{
			ShipmentID: hhg9.ID,
			Role:       models.RoleDESTINATION,
		},
	})
	hhg9.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg9.Move)

	/*
	 * Service member with delivered shipment
	 */
	email = "hhg@de.livered"

	offer10 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("3339dd2a-a23f-4967-a035-3bc9987c6848")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("2559dd2a-a23f-4967-a035-3bc9987c6824"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("ReadyForApprove"),
			Edipi:         models.StringPointer("4444567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("3442270d-4a6f-44ea-82f6-ae3cf3277c5d"),
			Locator:          "SCHNOO",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("466f8b8b-67a6-4ce3-b5c3-bd48c82512fc"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:           models.ShipmentStatusDELIVERED,
			ActualPickupDate: &nextValidMoveDateMinusFive,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			Accepted:                        models.BoolPointer(true),
		},
	})

	hhg10 := offer10.Shipment
	hhg10.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg10.Move)

	/*
	 * Service member with approved basics and accepted shipment to be approved
	 */
	email = "hhg@delivered.tocomplete"

	offer12 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("ab9fd68e-4461-4ba8-b630-9618b913e229")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("ab9fd68e-4461-4ba8-b630-9618b913e229"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("ReadyForApprove"),
			Edipi:         models.StringPointer("4444567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("abcd6b2f-9ef2-48be-b4ee-1c1e0a1456ef"),
			Locator:          "SSETZN",
			SelectedMoveType: &selectedMoveTypeHHG,
			Status:           models.MoveStatusAPPROVED,
		},
		Order: models.Order{
			OrdersNumber:        models.StringPointer("54321"),
			OrdersTypeDetail:    &typeDetail,
			DepartmentIndicator: models.StringPointer("AIR_FORCE"),
			TAC:                 models.StringPointer("99"),
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("ab7e2e3e-9bff-4bb0-b301-f97ad03350c1"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:           models.ShipmentStatusDELIVERED,
			ActualPickupDate: &nextValidMoveDateMinusFive,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			Accepted:                        models.BoolPointer(true),
		},
	})

	hhg12 := offer12.Shipment
	hhg12.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg12.Move)

	/*
	 * Service member with uploaded orders and an approved shipment
	 */
	email = "hhg@premo.ve"

	offer13 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("8f6b87f1-20ad-4c50-a855-ab66e222c7c3")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("1a98be36-5c4c-4056-b16f-d5a6c65b8569"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("Submitted"),
			Edipi:         models.StringPointer("4444567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("01d85649-18c2-44ad-854d-da8884579f42"),
			Locator:          "PREMVE",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("fd76c4fc-a2fb-45b6-a3a6-7c35357ab79a"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status: models.ShipmentStatusAWARDED,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
		},
	})

	hhg13 := offer13.Shipment
	hhg13.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg13.Move)

	/*
	 * Service member with uploaded orders and an approved shipment
	 */
	email = "hhg@dates.panel"

	offer14 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("444b87f1-20ad-4c50-a855-ab66e222c7c3")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("1222be36-5c4c-4056-b16f-d5a6c65b8569"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("Submitted"),
			Edipi:         models.StringPointer("4444567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("11185649-18c2-44ad-854d-da8884579f42"),
			Locator:          "DATESP",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("feeec4fc-a2fb-45b6-a3a6-7c35357ab79a"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:             models.ShipmentStatusAWARDED,
			ActualDeliveryDate: nil,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
		},
	})

	hhg14 := offer14.Shipment
	hhg14.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg14.Move)

	/* Service member with an in progress for doc testing on TSP side
	 */
	email = "doc.viewer@tsp.org"

	offer15 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("027f183d-a45e-44fc-b890-cd5092a99ecb")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("b5df04d6-2a35-4294-9a67-8a2427eba0bc"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("Submitted"),
			Edipi:         models.StringPointer("9999888777"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("ccd45bd4-660c-4ddd-b6c6-062da0a647f9"),
			Locator:          "DOCVWR",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("7ad595da-9b34-4914-aeaa-9a540d13872f"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:             models.ShipmentStatusAWARDED,
			ActualDeliveryDate: nil,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
		},
	})

	hhg15 := offer15.Shipment
	hhg15.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg15.Move)

	/* Service member with an in progress for testing delivery address
	 */
	email = "duty.station@tsp.org"

	offer16 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("2b6036ce-acf1-40fc-86da-d2b32329054f")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("815a314e-3c30-430b-bae7-54cf9ded79d4"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("Submitted"),
			Edipi:         models.StringPointer("9999888777"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("761743d9-2259-4bee-b144-3bda29311446"),
			Locator:          "DTYSTN",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("a36a58b4-51ab-4d39-bcdc-b3ca3a59a4a1"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:             models.ShipmentStatusAWARDED,
			HasDeliveryAddress: false,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
		},
	})

	hhg16 := offer16.Shipment
	hhg16.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg16.Move)

	/*
	 * Service member with a in progress for doc upload testing on TSP side
	 */
	email = "doc.upload@tsp.org"

	offer17 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("0d033463-09fd-498b-869d-30cda1c95599")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("98b21b35-9709-4da8-9d42-a2e887cf1e6c"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("Submitted"),
			Edipi:         models.StringPointer("9999888777"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("787c0921-7696-400e-86f5-c1bcb8bb88a3"),
			Locator:          "DOCUPL",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("ad3ee670-6978-46e1-bcfc-686cdd4ffa87"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			ID:                 uuid.FromStringOrNil("65e00326-420e-436a-89fc-6aeb3f90b870"),
			Status:             models.ShipmentStatusAWARDED,
			ActualDeliveryDate: nil,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
		},
	})

	hhg17 := offer17.Shipment
	hhg17.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg17.Move)

	/*
	 * Service member with approved basics and awarded shipment (can't approve shipment yet)
	 */
	email = "hhg@cant.approve"

	offer18 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("0187d7e5-2ee7-410a-b42f-d889a78b0bff")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("2f3ad6c4-2e6c-4c45-a7e5-8b220ebaabb6"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("BasicsApproveOnly"),
			Edipi:         models.StringPointer("4444567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("57e9275b-b433-474c-99f2-ac64966b3c9b"),
			Locator:          "BACON6",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		Order: models.Order{
			OrdersNumber:        models.StringPointer("54321"),
			OrdersTypeDetail:    &typeDetail,
			DepartmentIndicator: models.StringPointer("AIR_FORCE"),
			TAC:                 models.StringPointer("99"),
			SAC:                 models.StringPointer("SAC"),
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("cb49e75e-7897-4a01-8cff-c13ae85ca5ba"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status: models.ShipmentStatusAWARDED,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
		},
	})

	hhg18 := offer18.Shipment
	hhg18.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg18.Move)

	/*
	 * Service member with uploaded orders and an approved shipment
	 */
	email = "hhg@doc.uploads"

	offer19 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("5245b1ff-ae5a-4875-8a21-6b05c735b684")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("60cdcd83-6d6f-442f-a5b5-c256b312d000"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("Submitted"),
			Edipi:         models.StringPointer("4234567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("533d176f-0bab-4c51-88cd-c899f6855b9d"),
			Locator:          "BACON7",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("60e65f0c-aa21-4d95-a825-9d323a3dc4f1"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:             models.ShipmentStatusAWARDED,
			HasDeliveryAddress: true,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
		},
	})

	hhg19 := offer19.Shipment
	hhg19.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg19.Move)

	/*
	 * Service member with uploaded orders and an approved shipment. Use this to test zeroing dates.
	 */
	email = "hhg@dates.panel"

	offer20 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("cf1b1f09-8ea2-4f68-872e-a056c3a5f22f")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("6c4bc296-927c-4c6b-a01e-1f064c5d5f9b"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("Submitted"),
			Edipi:         models.StringPointer("4444567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("da9af941-253a-45e0-b012-8ee0385e28f8"),
			Locator:          "DATESZ",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("9728e6a1-0469-4718-9ba1-5d7baace1191"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:             models.ShipmentStatusAWARDED,
			ActualDeliveryDate: nil,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
		},
	})

	hhg20 := offer20.Shipment
	hhg20.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg20.Move)

	/* Service member with a doc for testing on TSP side
	 */
	email = "doc.owner@tsp.org"

	offer21 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("99bdaeed-a8a8-492e-9e28-7d0da6b1c907")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("61473913-36b8-425d-b46a-cee488a4ae71"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("Submitted"),
			Edipi:         models.StringPointer("2232332334"),
			PersonalEmail: models.StringPointer(email),
		},
		// These values should be populated for an approved move
		Order: models.Order{
			OrdersNumber:        models.StringPointer("12345"),
			OrdersTypeDetail:    &typeDetail,
			DepartmentIndicator: models.StringPointer("AIR_FORCE"),
			TAC:                 models.StringPointer("99"),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("60098ff1-8dc9-4318-a2e8-47bc8aac11a4"),
			Locator:          "GOTDOC",
			SelectedMoveType: &selectedMoveTypeHHG,
			Status:           models.MoveStatusAPPROVED,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("7ad595da-9b34-4914-aeaa-9a540d13872f"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:             models.ShipmentStatusAPPROVED,
			ActualDeliveryDate: nil,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			Accepted:                        models.BoolPointer(true),
		},
		Document: models.Document{
			ID:              uuid.FromStringOrNil("06886210-b151-4b15-951a-783d3d58f042"),
			ServiceMemberID: uuid.FromStringOrNil("61473913-36b8-425d-b46a-cee488a4ae71"),
		},
		MoveDocument: models.MoveDocument{
			ID:               uuid.FromStringOrNil("b660080d-0158-4214-99ca-216f82b26b3c"),
			DocumentID:       uuid.FromStringOrNil("06886210-b151-4b15-951a-783d3d58f042"),
			MoveDocumentType: "WEIGHT_TICKET",
			Status:           "OK",
			MoveID:           uuid.FromStringOrNil("60098ff1-8dc9-4318-a2e8-47bc8aac11a4"),
			Title:            "document_title",
		},
	})

	hhg21 := offer21.Shipment
	hhg21.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg21.Move)

	/*
	 * Service member with uploaded orders and an approved shipment with service agent
	 */
	email = "hhg@enter.premove"

	// Setting a weight estimate shows that even if PM survey is partially filled out,
	// the PM Survey Action Button still appears so long as there's no pm_survey_completed_at.
	weightEstimate := unit.Pound(5000)

	offer22 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("426b87f1-20ad-4c50-a855-ab66e222c7c3")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("4298be36-5c4c-4056-b16f-d5a6c65b8569"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("Approved"),
			Edipi:         models.StringPointer("4424567890"),
			PersonalEmail: models.StringPointer(email),
		},
		// These values should be populated for an approved move
		Order: models.Order{
			OrdersNumber:        models.StringPointer("12345"),
			OrdersTypeDetail:    &typeDetail,
			DepartmentIndicator: models.StringPointer("AIR_FORCE"),
			TAC:                 models.StringPointer("99"),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("42d85649-18c2-44ad-854d-da8884579f42"),
			Locator:          "ENTPMS",
			SelectedMoveType: &selectedMoveTypeHHG,
			Status:           models.MoveStatusAPPROVED,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("f426c4fc-a2fb-45b6-a3a6-7c35357ab79a"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:                 models.ShipmentStatusAPPROVED,
			PmSurveyWeightEstimate: &weightEstimate,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
		},
	})

	testdatagen.MakeServiceAgent(db, testdatagen.Assertions{
		ServiceAgent: models.ServiceAgent{
			Shipment:   &offer22.Shipment,
			ShipmentID: offer22.ShipmentID,
		},
	})

	hhg22 := offer22.Shipment
	hhg22.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg22.Move)

	/*
	 * Service member with accepted move but needs to be assigned a service agent
	 */
	email = "hhg@assign.serviceagent"
	offer23 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("8ff1c3ca-4c51-40ad-9926-8add5463eb25")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("e52c90df-502f-4fa2-8838-ee0894725b4d"),
			FirstName:     models.StringPointer("Assign"),
			LastName:      models.StringPointer("ServiceAgent"),
			Edipi:         models.StringPointer("4444567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("33686dbe-cd64-4786-8aaa-a93dda278683"),
			Locator:          "ASSIGN",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("d40edb7e-24c9-4a21-8e4b-2e473471263e"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status: models.ShipmentStatusACCEPTED,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
		},
	})
	hhg23 := offer23.Shipment
	hhg23.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg23.Move)

	/*
	 * Service member with in transit shipment
	 */
	email = "enter@delivery.date"

	netWeight := unit.Pound(2000)
	actualPickupDate := nextValidMoveDateMinusFive
	offer24 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("1af7ca19-8511-4c6e-a93b-144811c0fa7c")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("ae29e24b-b048-4c17-88d6-a008b91d0f85"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("ReadyForApprove"),
			Edipi:         models.StringPointer("4444567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("135af727-f570-4c7e-bf5b-878d717ef83c"),
			Locator:          "ENTDEL",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("ebfac7dc-acfa-4a88-bbbf-a2dd1a7f2657"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:           models.ShipmentStatusINTRANSIT,
			NetWeight:        &netWeight,
			ActualPickupDate: &actualPickupDate,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			Accepted:                        models.BoolPointer(true),
		},
	})

	authorizedStartDate := nextValidMoveDateMinusFive
	actualStartDate := nextValidMoveDateMinusFive
	testdatagen.MakeStorageInTransit(db, testdatagen.Assertions{
		StorageInTransit: models.StorageInTransit{
			ShipmentID:          offer24.ShipmentID,
			Shipment:            offer24.Shipment,
			EstimatedStartDate:  actualPickupDate,
			AuthorizedStartDate: &authorizedStartDate,
			ActualStartDate:     &actualStartDate,
			Status:              models.StorageInTransitStatusINSIT,
		},
	})

	hhg24 := offer24.Shipment
	hhg24.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg24.Move)

	/*
	 * Service member with a cancelled HHG move.
	 */
	email = "hhg@cancel.ed"

	hhg25 := testdatagen.MakeShipment(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("05ea5bc3-fd77-4f42-bdc5-a984a81b3829")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("d27bcb66-fc51-42b6-a13b-c896d34c79fb"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("Cancelled"),
			Edipi:         models.StringPointer("4444567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("da6bf1f4-a810-486d-befe-ddf8e9a4e2ef"),
			Locator:          "HHGCAN",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("d89dba9c-5ee9-40ee-8430-2e3eb13eedeb"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
	})

	hhg25.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg25.Move)
	hhg25.Move.Cancel("reasons")
	models.SaveMoveDependencies(db, &hhg25.Move)

	/*
	 * Service member with uploaded orders and an approved shipment to have weight added in the office app
	 */
	email = "hhg@addweights.office"

	offer26 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("611aea22-1689-4e16-90e7-e55d49010069")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("033297aa-4f4d-4df1-a05d-d22d717f6d5b"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("Submitted"),
			Edipi:         models.StringPointer("4444567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("2be4f6a3-82f5-4919-a257-39a24859058f"),
			Locator:          "WTSPNL",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("d2c24faf-3439-451f-b020-fc1492f6b4bf"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status: models.ShipmentStatusAWARDED,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
		},
	})

	hhg26 := offer26.Shipment
	hhg26.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg26.Move)

	/*
	* Service member to update dates from office app
	 */
	email = "hhg1@officeda.te"

	hhg27 := testdatagen.MakeShipment(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("5e2f7338-0f54-4ba9-99cc-796153da94f3")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("8cfe7777-d8a5-43ed-bb0e-5ba2ceda2251"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("Submitted"),
			Edipi:         models.StringPointer("4444555888"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("47e9c534-a93c-4986-ae8f-41fddefaa618"),
			Locator:          "ODATES",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("51004395-ecbf-4ab2-9edc-ec5041bbe390"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
	})

	hhg27.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg27.Move)

	/*
	 * Service member to update dates from office app
	 */
	email = "hhg2@officeda.te"

	hhg28 := testdatagen.MakeShipment(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("961108be-ace1-407c-b110-7e996e95d286")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("823a2177-3d68-43a5-a3ed-6b10454a6481"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("Submitted"),
			Edipi:         models.StringPointer("4444999888"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("762f2ec2-f362-4c14-b601-d7178c4862fe"),
			Locator:          "ODATE0",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("12e17ea7-9c94-4b61-a28d-5a81744a355c"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
	})
	hhg28.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg28.Move)

	/*
	 * Service member with uploaded orders and an approved shipment for adding a ppm
	 */
	email = "hhgforppm@award.ed"

	offer29 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("f83bc69f-10aa-48b7-b9fe-425b393d49b8")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("234960a6-2ccb-4914-8092-db58fc5d1d89"),
			FirstName:     models.StringPointer("HHG Ready"),
			LastName:      models.StringPointer("For PPM"),
			Edipi:         models.StringPointer("7777567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("3a98bf2e-fcca-4832-953b-022d4dd3814d"),
			Locator:          "COMBO1",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		Order: models.Order{
			IssueDate:        time.Date(testdatagen.TestYear, time.May, 20, 0, 0, 0, 0, time.UTC),
			HasDependents:    true,
			SpouseHasProGear: true,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("115f14f2-c982-4a54-a293-78935b61305d"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:             models.ShipmentStatusAWARDED,
			HasDeliveryAddress: true,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
		},
	})

	hhg29 := offer29.Shipment
	hhg29.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg29.Move)

	/*
	 * Service member with accepted shipment
	 */
	email = "hhgnotyet@approv.ed"

	offer30 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("edd11e8e-ebb3-4ed9-bd6c-69dd2ca2555f")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("547863b0-6757-4135-8e12-923a18a374ee"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("NotYetApproved"),
			Edipi:         models.StringPointer("4124567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("8bd3488c-b846-49ee-8a95-ed2de7f2f618"),
			Locator:          "ACC4PM",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("1524b6b5-5608-41cb-a814-ac1d9a427f42"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:             models.ShipmentStatusACCEPTED,
			HasDeliveryAddress: true,
			SourceGBLOC:        &sourceOffice.Gbloc,
			DestinationGBLOC:   &destOffice.Gbloc,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			Accepted:                        models.BoolPointer(true),
		},
	})

	hhg30 := offer30.Shipment

	testdatagen.MakeServiceAgent(db, testdatagen.Assertions{
		ServiceAgent: models.ServiceAgent{
			ShipmentID: hhg30.ID,
		},
	})
	testdatagen.MakeServiceAgent(db, testdatagen.Assertions{
		ServiceAgent: models.ServiceAgent{
			ShipmentID: hhg30.ID,
			Role:       models.RoleDESTINATION,
		},
	})
	hhg30.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg30.Move)

	/*
	 * Service member with approved shipment and pre move survey has already been filled out.
	 */
	email = "hhgalready@approv.ed"

	weightEstimate = unit.Pound(5000)

	offer31 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("f6cfe91f-c5f0-47dd-a796-5d9fb0f96289")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("c0abdde6-fe2c-4b0e-b0ed-860608eca04b"),
			FirstName:     models.StringPointer("HHGApproved"),
			LastName:      models.StringPointer("PMSurveyCompleted"),
			Edipi:         models.StringPointer("4124337809"),
			PersonalEmail: models.StringPointer(email),
		},
		// These values should be populated for an approved move
		Order: models.Order{
			OrdersNumber:        models.StringPointer("12345"),
			OrdersTypeDetail:    &typeDetail,
			DepartmentIndicator: models.StringPointer("AIR_FORCE"),
			TAC:                 models.StringPointer("99"),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("8c03b5a5-2ca5-49c1-a5a0-12f56c5f15c7"),
			Locator:          "APPPMS",
			SelectedMoveType: &selectedMoveTypeHHG,
			Status:           models.MoveStatusAPPROVED,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("e2351f50-9b07-4e6a-85eb-7c622486e859"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:             models.ShipmentStatusAPPROVED,
			HasDeliveryAddress: true,

			PmSurveyMethod:              "PHONE",
			PmSurveyPlannedPackDate:     &nextValidMoveDatePlusOne,
			PmSurveyPlannedPickupDate:   &nextValidMoveDatePlusFive,
			PmSurveyCompletedAt:         &nextValidMoveDatePlusOne,
			PmSurveyPlannedDeliveryDate: &nextValidMoveDatePlusTen,
			PmSurveyWeightEstimate:      &weightEstimate,
			SourceGBLOC:                 &sourceOffice.Gbloc,
			DestinationGBLOC:            &destOffice.Gbloc,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			Accepted:                        models.BoolPointer(true),
		},
	})

	hhg31 := offer31.Shipment

	testdatagen.MakeServiceAgent(db, testdatagen.Assertions{
		ServiceAgent: models.ServiceAgent{
			ShipmentID: hhg31.ID,
		},
	})
	testdatagen.MakeServiceAgent(db, testdatagen.Assertions{
		ServiceAgent: models.ServiceAgent{
			ShipmentID: hhg31.ID,
			Role:       models.RoleDESTINATION,
		},
	})
	hhg31.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg31.Move)

	/*
	 * Service member with approved basics and accepted shipment
	 */
	email = "hhg@gbl.disabled"

	offer32 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("ab04735c-fc9c-40a3-8e46-73dc0ca163da")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("35e3f64d-5615-4c9d-acf3-b832404d5627"),
			FirstName:     models.StringPointer("GBLDisabled"),
			LastName:      models.StringPointer("Waiting for office"),
			Edipi:         models.StringPointer("4444567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("262d079b-1e18-48bc-8351-f6a092af67d9"),
			Locator:          "GBLDIS",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		Order: models.Order{
			OrdersNumber:        models.StringPointer("54321"),
			OrdersTypeDetail:    &typeDetail,
			DepartmentIndicator: models.StringPointer("AIR_FORCE"),
			TAC:                 models.StringPointer("99"),
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("440dfbf6-a9da-4d37-b3e1-7327424eda01"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			ID:                          uuid.FromStringOrNil("53115e49-466e-42ee-85c6-9215add89cea"),
			Status:                      models.ShipmentStatusACCEPTED,
			PmSurveyConductedDate:       &nextValidMoveDatePlusOne,
			PmSurveyCompletedAt:         &nextValidMoveDatePlusOne,
			PmSurveyMethod:              "PHONE",
			PmSurveyPlannedPackDate:     &nextValidMoveDatePlusOne,
			PmSurveyPlannedPickupDate:   &nextValidMoveDatePlusOne,
			PmSurveyPlannedDeliveryDate: &nextValidMoveDatePlusOne,
			PmSurveyWeightEstimate:      &weightEstimate,
			SourceGBLOC:                 &sourceOffice.Gbloc,
			DestinationGBLOC:            &destOffice.Gbloc,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			Accepted:                        models.BoolPointer(true),
		},
	})

	testdatagen.MakeServiceAgent(db, testdatagen.Assertions{
		ServiceAgent: models.ServiceAgent{
			Shipment:   &offer32.Shipment,
			ShipmentID: offer32.ShipmentID,
		},
	})

	hhg32 := offer32.Shipment
	hhg32.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg32.Move)

	/*
	 * Service member with awarded shipment with no dependents and no spouse progear
	 */
	email = "nodepdents@nospousepro.gear"

	offer33 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("9a292af8-0e2e-4140-8bd2-165aa24d5071")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("9672cf74-bbe6-4cb2-97c8-f4c342d310a1"),
			FirstName:     models.StringPointer("No Dependents"),
			LastName:      models.StringPointer("No Spouse Progear"),
			Edipi:         models.StringPointer("4444567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Order: models.Order{
			HasDependents:    false,
			SpouseHasProGear: false,
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("3f8db9d7-db14-4f5c-a50b-0ae67ca3a225"),
			Locator:          "NDNSPG",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("89a1fe5a-0735-4d9f-8637-f1ec34081827"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:   models.ShipmentStatusAWARDED,
			BookDate: &nextValidMoveDate,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			Accepted:                        models.BoolPointer(true),
		},
	})

	hhg33 := offer33.Shipment
	hhg33.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg33.Move)

	/*
	 * Service member with accepted move but needs to be assigned an origin service agent
	 */
	email = "hhg@assignorigin.serviceagent"
	offer34 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("2a14af24-5448-414b-a114-e943d695a371")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("95ca8bc7-2fbd-46f7-871d-ddb3f8472b8d"),
			FirstName:     models.StringPointer("AssignOrigin"),
			LastName:      models.StringPointer("ServiceAgent"),
			Edipi:         models.StringPointer("4444512345"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("5bd2eb88-ca20-4678-8253-04da76db0a52"),
			Locator:          "ASNORG",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("0a93b301-1b66-4a22-ab0e-027ec19d81d9"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status: models.ShipmentStatusACCEPTED,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
		},
	})
	hhg34 := offer34.Shipment
	hhg34.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg34.Move)

	/*
	 * Service member with accepted move for use in testing SIT panel
	 */
	email = "hhg@sit.panel"
	offer35 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("08e0eb2c-d80f-494c-ae49-bd3497e41613")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("b6d43527-3fd2-4f04-936a-a69e7641a552"),
			FirstName:     models.StringPointer("SIT"),
			LastName:      models.StringPointer("Panel"),
			Edipi:         models.StringPointer("1357924680"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("b6d43527-3fd2-4f04-936a-a69e7641a552"),
			Locator:          "SITPAN",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("acb1a5f4-8750-4853-8db6-73b2acbe95d6"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status: models.ShipmentStatusACCEPTED,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			Accepted:                        models.BoolPointer(true),
		},
	})
	hhg35 := offer35.Shipment
	hhg35.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg35.Move)

	/*
	 * Service member with accepted move for use in testing SIT panel with existing SIT request from TSP
	 */
	email = "hhg@sit.requested.panel"
	offer36 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("05796fb0-3f9b-42bf-9873-ba60f23b50e2")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("dcd26e48-18a0-465a-ba36-1f71f6e5cccc"),
			FirstName:     models.StringPointer("SIT"),
			LastName:      models.StringPointer("Requested"),
			Edipi:         models.StringPointer("1357924680"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("34ab47e2-334e-423c-9b24-1d9ce118b1cb"),
			Locator:          "SITREQ",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("84d3a303-9b8b-4f95-8af4-d45e16aeb5c6"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status: models.ShipmentStatusACCEPTED,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			Accepted:                        models.BoolPointer(true),
		},
	})

	testdatagen.MakeStorageInTransit(db, testdatagen.Assertions{
		StorageInTransit: models.StorageInTransit{
			ShipmentID:         offer36.ShipmentID,
			Shipment:           offer36.Shipment,
			EstimatedStartDate: time.Date(2019, time.Month(3), 22, 0, 0, 0, 0, time.UTC),
		},
	})
	hhg36 := offer36.Shipment
	hhg36.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg36.Move)

	/*
	 * Service member with in-transit shipment and approved SIT to be placed in SIT
	 */
	email = "hhg@sit.approved"
	offer37 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("e558f9bc-5996-47dc-83fd-b8ee8bc78537")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("a6d36c52-f84e-46fd-b2d6-e9e56cfcc6b8"),
			FirstName:     models.StringPointer("SIT"),
			LastName:      models.StringPointer("Approved"),
			Edipi:         models.StringPointer("1357924680"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("98c8ba5a-92ed-4e1d-b669-9dd41243b615"),
			Locator:          "SITAPR",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("0e036dff-5843-48ce-bcd5-1b248635c1bd"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status: models.ShipmentStatusINTRANSIT,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			Accepted:                        models.BoolPointer(true),
		},
	})

	authorizedStartDate = time.Date(2019, time.Month(3), 26, 0, 0, 0, 0, time.UTC)
	testdatagen.MakeStorageInTransit(db, testdatagen.Assertions{
		StorageInTransit: models.StorageInTransit{
			ShipmentID:          offer37.ShipmentID,
			Shipment:            offer37.Shipment,
			Status:              models.StorageInTransitStatusAPPROVED,
			EstimatedStartDate:  time.Date(2019, time.Month(3), 22, 0, 0, 0, 0, time.UTC),
			AuthorizedStartDate: &authorizedStartDate,
		},
	})
	hhg37 := offer37.Shipment
	hhg37.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg37.Move)

	/*
	 * Service member with accepted move for use in testing SIT Denial by the office
	 */
	email = "hhg@sit.requested.todeny.panel"
	offer38 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("fc3fe543-589d-4a91-ab03-8bd8bdfc7df3")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("db6d867f-4981-4163-b683-7e3210a0a246"),
			FirstName:     models.StringPointer("SIT"),
			LastName:      models.StringPointer("RequestedToDeny"),
			Edipi:         models.StringPointer("1357924680"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("51971a9d-db4c-4fda-84dc-1099efc3b3a3"),
			Locator:          "SITDEN",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("a392b7e2-d15c-4c20-9571-0bed93f8ea5f"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status: models.ShipmentStatusACCEPTED,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			Accepted:                        models.BoolPointer(true),
		},
	})

	testdatagen.MakeStorageInTransit(db, testdatagen.Assertions{
		StorageInTransit: models.StorageInTransit{
			ShipmentID:         offer38.ShipmentID,
			Shipment:           offer38.Shipment,
			EstimatedStartDate: time.Date(2019, time.Month(3), 22, 0, 0, 0, 0, time.UTC),
		},
	})
	hhg38 := offer38.Shipment
	hhg38.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg38.Move)

	/*
	 * Service member with in-transit move for use in testing "in sit" entitlement remaining days
	 * (for both office and TSP)
	 */
	email = "hhg@sit.insit"
	offer39 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("367a6772-e5b6-477e-b1d9-938439b56c00")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("47817aa2-d30c-4f53-8f7d-abd14a000ebb"),
			FirstName:     models.StringPointer("SIT"),
			LastName:      models.StringPointer("InSIT"),
			Edipi:         models.StringPointer("1357924680"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("3b351688-6108-4d3b-9cc0-8a6a1250cda3"),
			Locator:          "SITIN1",
			SelectedMoveType: &selectedMoveTypeHHG,
			Status:           models.MoveStatusAPPROVED,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("bbf23676-ea22-4432-9627-89c27dffd9a7"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status: models.ShipmentStatusINTRANSIT,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			Accepted:                        models.BoolPointer(true),
		},
	})

	testdatagen.MakeStorageInTransit(db, testdatagen.Assertions{
		StorageInTransit: models.StorageInTransit{
			ShipmentID:          offer39.ShipmentID,
			Shipment:            offer39.Shipment,
			Status:              models.StorageInTransitStatusINSIT,
			EstimatedStartDate:  time.Date(2019, time.Month(3), 29, 0, 0, 0, 0, time.UTC),
			AuthorizedStartDate: swag.Time(time.Date(2019, time.Month(3), 29, 0, 0, 0, 0, time.UTC)),
			ActualStartDate:     swag.Time(time.Date(2019, time.Month(3), 30, 0, 0, 0, 0, time.UTC)),
		},
	})
	hhg39 := offer39.Shipment
	hhg39.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg39.Move)

	/*
	 * HHG40
	 * Service member with in-transit shipment and Origin InSIT SIT
	 */
	email = "hhg@sit.insit.origin"
	offer40 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.NewV4()),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.Must(uuid.NewV4()),
			FirstName:     models.StringPointer("ORIGIN-SIT"),
			LastName:      models.StringPointer("InSIT"),
			Edipi:         models.StringPointer("1857924699"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.Must(uuid.NewV4()),
			Locator:          "SITOIN", // SIT Origin InSIT
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.Must(uuid.NewV4()),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status: models.ShipmentStatusINTRANSIT,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			Accepted:                        models.BoolPointer(true),
		},
	})

	authorizedStartDateOffer40 := time.Date(2019, time.Month(3), 26, 0, 0, 0, 0, time.UTC)
	sit40 := models.StorageInTransit{
		ID:                  uuid.Must(uuid.NewV4()),
		ShipmentID:          offer40.ShipmentID,
		Shipment:            offer40.Shipment,
		Location:            models.StorageInTransitLocationORIGIN,
		Status:              models.StorageInTransitStatusINSIT,
		EstimatedStartDate:  time.Date(2019, time.Month(3), 22, 0, 0, 0, 0, time.UTC),
		ActualStartDate:     &authorizedStartDateOffer40,
		AuthorizedStartDate: &authorizedStartDateOffer40,
		SITNumber:           models.StringPointer("400000001"),
	}
	testdatagen.MakeStorageInTransit(db, testdatagen.Assertions{
		StorageInTransit: sit40,
	})
	hhg40 := offer40.Shipment
	hhg40.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg40.Move)

	/*
	 * HHG41
	 * Service member with delivered Shipment and Origin InSIT SIT (SIT added after shipment is delivered)
	 */

	// Info used to create a delivered shipment ready for invoice
	shipmentHhgParams41 := hhgReadyToInvoiceParams{
		TspUser:                tspUser,
		Logger:                 logger,
		Storer:                 storer,
		Email:                  "hhg@delivered.insit.origin",
		NetWeight:              3000,
		WeightEstimate:         5000,
		SourceGBLOC:            "ABCD",
		DestGBLOC:              "QRED",
		GBLNumber:              "010041",
		ServiceMemberFirstName: "Origin-SIT",
		ServiceMemberLastName:  "ShipmentDelivered",
		Locator:                "DISIT1",
		PlannerDistance:        1234,
	}

	// create and return a delivered shipment ready for invoice
	hhg41 := makeHhgReadyToInvoiceWithSIT(db, shipmentHhgParams41)

	// Add SIT to shipment after shipment is delivered
	// Create and add to shipment a SIT at Origin that is IN-SIT
	sitID41 := uuid.Must(uuid.NewV4())
	authorizedStartDateOffer41 := time.Date(2019, time.Month(3), 26, 0, 0, 0, 0, time.UTC)
	sit41 := models.StorageInTransit{
		ID:                  sitID41,
		ShipmentID:          hhg41.ID,
		Shipment:            hhg41,
		Location:            models.StorageInTransitLocationORIGIN,
		Status:              models.StorageInTransitStatusINSIT,
		EstimatedStartDate:  authorizedStartDateOffer41,
		AuthorizedStartDate: &authorizedStartDateOffer41,
		ActualStartDate:     &authorizedStartDateOffer41,
		SITNumber:           models.StringPointer("410000001"),
	}
	testdatagen.MakeStorageInTransit(db, testdatagen.Assertions{
		StorageInTransit: sit41,
	})

	/*
	 * HHG42
	 * Service member with delivered Shipment and Origin InSIT SIT (SIT added while shipment is in Transit)
	 */

	// Info used to create a delivered shipment ready for invoice
	shipmentHhgParams42 := hhgReadyToInvoiceParams{
		TspUser:                tspUser,
		Logger:                 logger,
		Storer:                 storer,
		Email:                  "hhg@delivered.insit.origin2",
		NetWeight:              3000,
		WeightEstimate:         5000,
		SourceGBLOC:            "ABCD",
		DestGBLOC:              "QRED",
		GBLNumber:              "010042",
		ServiceMemberFirstName: "Origin-SIT",
		ServiceMemberLastName:  "ShipmentDelivered",
		Locator:                "DISIT2",
		PlannerDistance:        1234,
	}

	authorizedStartDate42 := time.Date(2019, time.Month(3), 26, 0, 0, 0, 0, time.UTC)
	sitID42 := uuid.Must(uuid.NewV4())
	sit42 := models.StorageInTransit{
		ID:                  sitID42,
		Location:            models.StorageInTransitLocationORIGIN,
		Status:              models.StorageInTransitStatusAPPROVED,
		EstimatedStartDate:  authorizedStartDate42,
		AuthorizedStartDate: &authorizedStartDate42,
	}

	shipmentHhgParams42.SITs = append(shipmentHhgParams42.SITs, sit42)

	// create and return a delivered shipment ready for invoice
	hhg42 := makeHhgReadyToInvoiceWithSIT(db, shipmentHhgParams42)

	// Transition SIT to InSIT/IN_SIT
	placeInSITParams42 := placeInSITParams{
		SITID:           sit42.ID,
		ShipmentID:      hhg42.ID,
		Shipment:        hhg42,
		ActualStartDate: *sit42.AuthorizedStartDate,
	}
	sitPlaceInSIT(db, placeInSITParams42, tspUserSession)

	/*
	 * HHG43
	 * Service member with in-transit move and denied SIT
	 */
	email = "hhg@sit.denied"
	offer43 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("b46a5aa9-c923-4a85-9d00-215cd2e1c62b")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("aa404f76-cba2-47ac-98b8-c6bc7c623e6f"),
			FirstName:     models.StringPointer("SIT"),
			LastName:      models.StringPointer("Denied"),
			Edipi:         models.StringPointer("1357924680"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("238c0f0a-22d4-4868-8b9c-b0fee514ce61"),
			Locator:          "SITDN2",
			SelectedMoveType: &selectedMoveTypeHHG,
			Status:           models.MoveStatusAPPROVED,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("d46a0c60-1f83-48b0-8993-18430f8b4bcf"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status: models.ShipmentStatusINTRANSIT,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			Accepted:                        models.BoolPointer(true),
		},
	})

	testdatagen.MakeStorageInTransit(db, testdatagen.Assertions{
		StorageInTransit: models.StorageInTransit{
			ShipmentID:         offer43.ShipmentID,
			Shipment:           offer43.Shipment,
			Status:             models.StorageInTransitStatusDENIED,
			EstimatedStartDate: time.Date(2019, time.Month(3), 29, 0, 0, 0, 0, time.UTC),
		},
	})
	hhg43 := offer43.Shipment
	hhg43.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg43.Move)

	/*
	 * Service member with accepted move for use in testing the deletion of SIT
	 */
	email = "hhg@sit.todelete"
	offer44 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("76616087-713d-4837-8941-f2b73f532a10")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("54bae5b4-af77-4693-a3b3-3ff6f5796a92"),
			FirstName:     models.StringPointer("SIT"),
			LastName:      models.StringPointer("ToDelete"),
			Edipi:         models.StringPointer("1357924622"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("9811bc2c-45e7-40d2-aee0-d863f0d2b7ee"),
			Locator:          "SITDEL",
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("0c395d75-ec2f-45de-8d3f-74716d69aa67"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status: models.ShipmentStatusACCEPTED,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			Accepted:                        models.BoolPointer(true),
		},
	})

	testdatagen.MakeStorageInTransit(db, testdatagen.Assertions{
		StorageInTransit: models.StorageInTransit{
			ShipmentID:         offer44.ShipmentID,
			Shipment:           offer44.Shipment,
			EstimatedStartDate: time.Date(2019, time.Month(4), 22, 0, 0, 0, 0, time.UTC),
		},
	})
	hhg44 := offer44.Shipment
	hhg44.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg44.Move)

	/*
	 * HHG45
	 * Service member with in-transit shipment and Origin DELIVERED SIT
	 */
	email = "hhg@sit.delivered.origin"
	offer45 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.NewV4()),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.Must(uuid.NewV4()),
			FirstName:     models.StringPointer("ORIGIN-SIT"),
			LastName:      models.StringPointer("DELIVERED"),
			Edipi:         models.StringPointer("1857924699"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.Must(uuid.NewV4()),
			Locator:          "SITDLV", // SIT Origin DELIVERED
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.Must(uuid.NewV4()),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status: models.ShipmentStatusDELIVERED,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			Accepted:                        models.BoolPointer(true),
		},
	})

	authorizedStartDateOffer45 := time.Date(2019, time.Month(3), 26, 0, 0, 0, 0, time.UTC)
	outDate45 := time.Date(2019, time.Month(3), 27, 0, 0, 0, 0, time.UTC)
	sit45 := models.StorageInTransit{
		ID:                  uuid.Must(uuid.NewV4()),
		ShipmentID:          offer45.ShipmentID,
		Shipment:            offer45.Shipment,
		Location:            models.StorageInTransitLocationORIGIN,
		Status:              models.StorageInTransitStatusDELIVERED,
		EstimatedStartDate:  time.Date(2019, time.Month(3), 22, 0, 0, 0, 0, time.UTC),
		ActualStartDate:     &authorizedStartDateOffer45,
		AuthorizedStartDate: &authorizedStartDateOffer45,
		OutDate:             &outDate45,
		SITNumber:           models.StringPointer("400000001"),
	}
	testdatagen.MakeStorageInTransit(db, testdatagen.Assertions{
		StorageInTransit: sit45,
	})
	hhg45 := offer45.Shipment
	hhg45.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg45.Move)

	/* HHG46
	 * Service member with in transit shipment and SIT less than 30 mi
	 */
	email46 := "enter@delivery.sit30"

	pickupAddress := models.Address{
		StreetAddress1: "9611 Highridge Dr",
		StreetAddress2: swag.String("P.O. Box 12345"),
		StreetAddress3: swag.String("c/o Some Person"),
		City:           "Beverly Hills",
		State:          "CA",
		PostalCode:     "90210",
		Country:        swag.String("US"),
	}
	pickupAddress = testdatagen.MakeAddress(db, testdatagen.Assertions{
		Address: pickupAddress,
	})

	destAddress := models.Address{
		StreetAddress1: "2157 Willhaven Dr ",
		StreetAddress2: swag.String(""),
		StreetAddress3: swag.String(""),
		City:           "Augusta",
		State:          "GA",
		PostalCode:     "30909",
		Country:        swag.String("US"),
	}
	destAddress = testdatagen.MakeAddress(db, testdatagen.Assertions{
		Address: destAddress,
	})

	sitOriginAddress := models.Address{
		StreetAddress1: "1860 Vine St",
		StreetAddress2: swag.String(""),
		StreetAddress3: swag.String(""),
		City:           "Los Angeles",
		State:          "CA",
		PostalCode:     "90028",
		Country:        swag.String("US"),
	}
	sitOriginAddress = testdatagen.MakeAddress(db, testdatagen.Assertions{
		Address: sitOriginAddress,
	})

	sitDestinationAddress46 := models.Address{
		StreetAddress1: "1045 Bertram Rd",
		StreetAddress2: swag.String(""),
		StreetAddress3: swag.String(""),
		City:           "Augusta",
		State:          "GA",
		PostalCode:     "30909",
		Country:        swag.String("US"),
	}
	sitDestinationAddress46 = testdatagen.MakeAddress(db, testdatagen.Assertions{
		Address: sitDestinationAddress46,
	})

	netWeight46 := unit.Pound(2000)
	actualPickupDate46 := nextValidMoveDate
	offer46 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("b742b1a8-3900-4a9b-aad8-af496a7b43b3")),
			LoginGovEmail: email46,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("1ff3f8f1-5381-4ab1-9b5b-46cc181d6abf"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("ReadyForDelivery"),
			Edipi:         models.StringPointer("4544567890"),
			PersonalEmail: models.StringPointer(email46),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("2fcda8df-d913-460c-9cc2-c18105fae7fa"),
			Locator:          "SIT030",
			SelectedMoveType: &selectedMoveTypeHHG,
			Orders: models.Order{
				NewDutyStation: models.DutyStation{
					Address: destAddress,
				},
			},
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("8003d039-d692-4c6b-9d5f-3fd04494edf0"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:           models.ShipmentStatusINTRANSIT,
			NetWeight:        &netWeight46,
			ActualPickupDate: &actualPickupDate46,
			PickupAddress:    &pickupAddress,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			Accepted:                        models.BoolPointer(true),
		},
	})

	hhg46 := offer46.Shipment
	sitID46Destination := uuid.Must(uuid.NewV4())
	authorizedStartDateOffer46Dest := time.Date(2019, time.Month(3), 26, 0, 0, 0, 0, time.UTC)
	sit46Destination := models.StorageInTransit{
		ID:                  sitID46Destination,
		ShipmentID:          hhg46.ID,
		Shipment:            hhg46,
		Location:            models.StorageInTransitLocationDESTINATION,
		Status:              models.StorageInTransitStatusAPPROVED,
		EstimatedStartDate:  authorizedStartDateOffer46Dest,
		AuthorizedStartDate: &authorizedStartDateOffer46Dest,
		ActualStartDate:     &authorizedStartDateOffer46Dest,
		WarehouseID:         "450384",
		WarehouseName:       "Iron Guard Storage",
		WarehouseAddress:    sitDestinationAddress46,
	}
	testdatagen.MakeStorageInTransit(db, testdatagen.Assertions{
		StorageInTransit: sit46Destination,
	})

	// Transition SIT to InSIT/IN_SIT
	placeInSITParams46 := placeInSITParams{
		SITID:           sit46Destination.ID,
		ShipmentID:      hhg46.ID,
		Shipment:        hhg46,
		ActualStartDate: *sit46Destination.AuthorizedStartDate,
	}
	sitPlaceInSIT(db, placeInSITParams46, tspUserSession)

	sitID46Origin := uuid.Must(uuid.NewV4())
	authorizedStartDateOffer46Orig := time.Date(2019, time.Month(3), 2, 0, 0, 0, 0, time.UTC)
	sit46Origin := models.StorageInTransit{
		ID:                  sitID46Origin,
		ShipmentID:          hhg46.ID,
		Shipment:            hhg46,
		Location:            models.StorageInTransitLocationORIGIN,
		Status:              models.StorageInTransitStatusAPPROVED,
		EstimatedStartDate:  authorizedStartDateOffer46Orig,
		AuthorizedStartDate: &authorizedStartDateOffer46Orig,
		ActualStartDate:     &authorizedStartDateOffer46Orig,
		WarehouseID:         "450383",
		WarehouseName:       "Extra Space Storage",
		WarehouseAddress:    sitOriginAddress,
	}
	testdatagen.MakeStorageInTransit(db, testdatagen.Assertions{
		StorageInTransit: sit46Origin,
	})

	hhg46.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg46.Move)

	// Transition SIT to InSIT/IN_SIT
	placeInSITParams46 = placeInSITParams{
		SITID:           sit46Origin.ID,
		ShipmentID:      hhg46.ID,
		Shipment:        hhg46,
		ActualStartDate: *sit46Origin.AuthorizedStartDate,
	}
	sitPlaceInSIT(db, placeInSITParams46, tspUserSession)

	/* HHG47
	 * Service member with in transit shipment and SIT less than or equal to 50 mi
	 */
	email47 := "enter@delivery.sit50"

	pickupAddress47 := models.Address{
		StreetAddress1: "9611 Highridge Dr",
		StreetAddress2: swag.String("P.O. Box 12345"),
		StreetAddress3: swag.String("c/o Some Person"),
		City:           "Beverly Hills",
		State:          "CA",
		PostalCode:     "90210",
		Country:        swag.String("US"),
	}
	pickupAddress47 = testdatagen.MakeAddress(db, testdatagen.Assertions{
		Address: pickupAddress47,
	})

	destAddress47 := models.Address{
		StreetAddress1: "2157 Willhaven Dr ",
		StreetAddress2: swag.String(""),
		StreetAddress3: swag.String(""),
		City:           "Augusta",
		State:          "GA",
		PostalCode:     "30909",
		Country:        swag.String("US"),
	}
	destAddress47 = testdatagen.MakeAddress(db, testdatagen.Assertions{
		Address: destAddress47,
	})

	//Extra Space Storage, 155 S Adams St, Anaheim, CA 92802: less than 50 depending on route
	//mileage is: 39.7mi, 47.5mi, and 49.2mi
	sitOriginAddress47 := models.Address{
		StreetAddress1: "155 S Adams St",
		StreetAddress2: swag.String(""),
		StreetAddress3: swag.String(""),
		City:           "Anaheim",
		State:          "CA",
		PostalCode:     "92802",
		Country:        swag.String("US"),
	}
	sitOriginAddress47 = testdatagen.MakeAddress(db, testdatagen.Assertions{
		Address: sitOriginAddress47,
	})

	netWeight47 := unit.Pound(2000)
	actualPickupDate47 := nextValidMoveDate
	offer47 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.NewV4()),
			LoginGovEmail: email47,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.Must(uuid.NewV4()),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("ReadyForDelivery"),
			Edipi:         models.StringPointer("4744567890"),
			PersonalEmail: models.StringPointer(email47),
		},
		Move: models.Move{
			ID:               uuid.Must(uuid.NewV4()),
			Locator:          "SIT050",
			SelectedMoveType: &selectedMoveTypeHHG,
			Orders: models.Order{
				NewDutyStation: models.DutyStation{
					Address: destAddress,
				},
			},
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.Must(uuid.NewV4()),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:           models.ShipmentStatusINTRANSIT,
			NetWeight:        &netWeight47,
			ActualPickupDate: &actualPickupDate47,
			PickupAddress:    &pickupAddress,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			Accepted:                        models.BoolPointer(true),
		},
	})

	hhg47 := offer47.Shipment
	sitID47Origin := uuid.Must(uuid.NewV4())
	authorizedStartDateOffer47Orig := time.Date(2019, time.Month(3), 2, 0, 0, 0, 0, time.UTC)
	sit47Origin := models.StorageInTransit{
		ID:                  sitID47Origin,
		ShipmentID:          hhg47.ID,
		Shipment:            hhg47,
		Location:            models.StorageInTransitLocationORIGIN,
		Status:              models.StorageInTransitStatusAPPROVED,
		EstimatedStartDate:  authorizedStartDateOffer47Orig,
		AuthorizedStartDate: &authorizedStartDateOffer47Orig,
		ActualStartDate:     &authorizedStartDateOffer47Orig,
		WarehouseID:         "450383",
		WarehouseName:       "Extra Space Storage",
		WarehouseAddress:    sitOriginAddress47,
	}
	testdatagen.MakeStorageInTransit(db, testdatagen.Assertions{
		StorageInTransit: sit47Origin,
	})

	hhg47.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg47.Move)

	// Transition SIT to InSIT/IN_SIT
	placeInSITParams47 := placeInSITParams{
		SITID:           sit47Origin.ID,
		ShipmentID:      hhg47.ID,
		Shipment:        hhg47,
		ActualStartDate: *sit47Origin.AuthorizedStartDate,
	}
	sitPlaceInSIT(db, placeInSITParams47, tspUserSession)

	/* HHG48
	 * Service member with in transit shipment and SIT more than 50 mi
	 */

	email48 := "enter@delivery.sit51"

	pickupAddress48 := models.Address{
		StreetAddress1: "9611 Highridge Dr",
		StreetAddress2: swag.String(""),
		StreetAddress3: swag.String(""),
		City:           "Beverly Hills",
		State:          "CA",
		PostalCode:     "90210",
		Country:        swag.String("US"),
	}
	pickupAddress48 = testdatagen.MakeAddress(db, testdatagen.Assertions{
		Address: pickupAddress48,
	})

	destAddress48 := models.Address{
		StreetAddress1: "2157 Willhaven Dr ",
		StreetAddress2: swag.String(""),
		StreetAddress3: swag.String(""),
		City:           "Augusta",
		State:          "GA",
		PostalCode:     "30909",
		Country:        swag.String("US"),
	}
	destAddress48 = testdatagen.MakeAddress(db, testdatagen.Assertions{
		Address: destAddress48,
	})

	// Southern Storage
	// 1177 US-29, Valley, AL 36854
	// more than 200 mi from destination
	sitDestinationAddress48 := models.Address{
		StreetAddress1: "1177 US-29",
		StreetAddress2: swag.String(""),
		StreetAddress3: swag.String(""),
		City:           "Valley",
		State:          "AL",
		PostalCode:     "36854",
		Country:        swag.String("US"),
	}
	sitDestinationAddress48 = testdatagen.MakeAddress(db, testdatagen.Assertions{
		Address: sitDestinationAddress48,
	})

	netWeight48 := unit.Pound(2000)
	actualPickupDate48 := nextValidMoveDate
	offer48 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.NewV4()),
			LoginGovEmail: email48,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.Must(uuid.NewV4()),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("ReadyForDelivery"),
			Edipi:         models.StringPointer("4844567890"),
			PersonalEmail: models.StringPointer(email48),
		},
		Move: models.Move{
			ID:               uuid.Must(uuid.NewV4()),
			Locator:          "SIT051",
			SelectedMoveType: &selectedMoveTypeHHG,
			Orders: models.Order{
				NewDutyStation: models.DutyStation{
					Address: destAddress,
				},
			},
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.Must(uuid.NewV4()),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:           models.ShipmentStatusINTRANSIT,
			NetWeight:        &netWeight48,
			ActualPickupDate: &actualPickupDate48,
			PickupAddress:    &pickupAddress,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			Accepted:                        models.BoolPointer(true),
		},
	})

	hhg48 := offer48.Shipment
	sitID48Destination := uuid.Must(uuid.NewV4())
	authorizedStartDateOffer48Dest := time.Date(2019, time.Month(3), 26, 0, 0, 0, 0, time.UTC)
	sit48Destination := models.StorageInTransit{
		ID:                  sitID48Destination,
		ShipmentID:          hhg48.ID,
		Shipment:            hhg48,
		Location:            models.StorageInTransitLocationDESTINATION,
		Status:              models.StorageInTransitStatusAPPROVED,
		EstimatedStartDate:  authorizedStartDateOffer48Dest,
		AuthorizedStartDate: &authorizedStartDateOffer48Dest,
		ActualStartDate:     &authorizedStartDateOffer48Dest,
		WarehouseID:         "450384",
		WarehouseName:       "Iron Guard Storage",
		WarehouseAddress:    sitDestinationAddress48,
	}
	testdatagen.MakeStorageInTransit(db, testdatagen.Assertions{
		StorageInTransit: sit48Destination,
	})

	// Transition SIT to InSIT/IN_SIT
	placeInSITParams48 := placeInSITParams{
		SITID:           sit48Destination.ID,
		ShipmentID:      hhg48.ID,
		Shipment:        hhg48,
		ActualStartDate: *sit48Destination.AuthorizedStartDate,
	}
	sitPlaceInSIT(db, placeInSITParams48, tspUserSession)

	hhg48.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg48.Move)

	/*
	 * HHG49
	 * Service member with in-transit shipment and Destination DELIVERED SIT
	 */
	email = "hhg@sit.delivered.destination"
	offer49 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.NewV4()),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.Must(uuid.NewV4()),
			FirstName:     models.StringPointer("DESTINATION-SIT"),
			LastName:      models.StringPointer("DELIVERED"),
			Edipi:         models.StringPointer("1857924699"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.Must(uuid.NewV4()),
			Locator:          "SITDST", // SIT Destination DELIVERED
			SelectedMoveType: &selectedMoveTypeHHG,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.Must(uuid.NewV4()),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status: models.ShipmentStatusDELIVERED,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			Accepted:                        models.BoolPointer(true),
		},
	})

	authorizedStartDateOffer49 := time.Date(2019, time.Month(3), 26, 0, 0, 0, 0, time.UTC)
	outDate49 := time.Date(2019, time.Month(3), 27, 0, 0, 0, 0, time.UTC)
	sit49 := models.StorageInTransit{
		ID:                  uuid.Must(uuid.NewV4()),
		ShipmentID:          offer49.ShipmentID,
		Shipment:            offer49.Shipment,
		Location:            models.StorageInTransitLocationDESTINATION,
		Status:              models.StorageInTransitStatusDELIVERED,
		EstimatedStartDate:  time.Date(2019, time.Month(3), 22, 0, 0, 0, 0, time.UTC),
		ActualStartDate:     &authorizedStartDateOffer49,
		AuthorizedStartDate: &authorizedStartDateOffer49,
		OutDate:             &outDate49,
		SITNumber:           models.StringPointer("400000001"),
	}
	testdatagen.MakeStorageInTransit(db, testdatagen.Assertions{
		StorageInTransit: sit49,
	})
	hhg49 := offer49.Shipment
	hhg49.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg49.Move)

	/*
	 * Service member with a ppm ready to request payment
	 */
	email = "ppm@requestingpay.ment"
	uuidStr = "8e0d7e98-134e-4b28-bdd1-7d6b1ff34f9e"
	testdatagen.MakeUser(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString(uuidStr)),
			LoginGovEmail: email,
		},
	})
	ppm5 := testdatagen.MakePPM(db, testdatagen.Assertions{
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("ff1f56c0-544e-4109-8168-f91ebcbbb878"),
			UserID:        uuid.FromStringOrNil(uuidStr),
			FirstName:     models.StringPointer("PPM"),
			LastName:      models.StringPointer("RequestingPay"),
			Edipi:         models.StringPointer("6737033988"),
			PersonalEmail: models.StringPointer(email),
		},
		// These values should be populated for an approved move
		Order: models.Order{
			OrdersNumber:        models.StringPointer("62341"),
			OrdersTypeDetail:    &typeDetail,
			DepartmentIndicator: models.StringPointer("AIR_FORCE"),
			TAC:                 models.StringPointer("99"),
		},
		Move: models.Move{
			ID:      uuid.FromStringOrNil("946a5d40-0636-418f-b457-474915fb0149"),
			Locator: "REQPAY",
		},
		PersonallyProcuredMove: models.PersonallyProcuredMove{
			OriginalMoveDate: &pastTime,
		},
		Uploader: loader,
	})
	ppm5.Move.Submit(time.Now())
	ppm5.Move.Approve()
	// This is the same PPM model as ppm5, but this is the one that will be saved by SaveMoveDependencies
	ppm5.Move.PersonallyProcuredMoves[0].Submit(time.Now())
	ppm5.Move.PersonallyProcuredMoves[0].Approve(time.Now())
	models.SaveMoveDependencies(db, &ppm5.Move)

	/*
	 * Service member with a ppm move approved, but not in progress
	 */
	email = "ppm@approv.ed"
	uuidStr = "70665111-7bbb-4876-a53d-18bb125c943e"
	testdatagen.MakeUser(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString(uuidStr)),
			LoginGovEmail: email,
		},
	})
	inProgressDate := nextValidMoveDatePlusTen
	typeDetails := internalmessages.OrdersTypeDetailPCSTDY
	ppmApproved := testdatagen.MakePPM(db, testdatagen.Assertions{
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("acfed739-9e7a-4d95-9a56-698ef0392500"),
			UserID:        uuid.FromStringOrNil(uuidStr),
			FirstName:     models.StringPointer("PPM"),
			LastName:      models.StringPointer("Approved"),
			Edipi:         models.StringPointer("7617044099"),
			PersonalEmail: models.StringPointer(email),
		},
		// These values should be populated for an approved move
		Order: models.Order{
			OrdersNumber:        models.StringPointer("12345"),
			OrdersTypeDetail:    &typeDetails,
			DepartmentIndicator: models.StringPointer("AIR_FORCE"),
			TAC:                 models.StringPointer("99"),
		},
		Move: models.Move{
			ID:      uuid.FromStringOrNil("bd3d46b3-cb76-40d5-a622-6ada239e5504"),
			Locator: "APPROV",
		},
		PersonallyProcuredMove: models.PersonallyProcuredMove{
			OriginalMoveDate: &inProgressDate,
		},
		Uploader: loader,
	})
	ppmApproved.Move.Submit(time.Now())
	ppmApproved.Move.Approve()
	// This is the same PPM model as ppm2, but this is the one that will be saved by SaveMoveDependencies
	ppmApproved.Move.PersonallyProcuredMoves[0].Submit(time.Now())
	ppmApproved.Move.PersonallyProcuredMoves[0].Approve(time.Now())
	models.SaveMoveDependencies(db, &ppmApproved.Move)

	/*
	 * Another service member with orders and a move
	 */
	email = "profile@co.mple.te"
	uuidStr = "99360a51-8cfa-4e25-ae57-24e66077305f"
	testdatagen.MakeUser(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString(uuidStr)),
			LoginGovEmail: email,
		},
	})

	testdatagen.MakeMove(db, testdatagen.Assertions{
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("2672baac-53a1-4767-b4a3-976e53cc224e"),
			UserID:        uuid.FromStringOrNil(uuidStr),
			FirstName:     models.StringPointer("Another Profile"),
			LastName:      models.StringPointer("Complete"),
			Edipi:         models.StringPointer("8893105161"),
			PersonalEmail: models.StringPointer(email),
		},
		Order: models.Order{
			HasDependents:    true,
			SpouseHasProGear: true,
		},
		Move: models.Move{
			ID:      uuid.FromStringOrNil("6f6ac599-e23f-43af-9b83-5d75a78e933f"),
			Locator: "COMPLE",
		},
		Uploader: loader,
	})

	/*
	* Creates a valid, unclaimed access code
	 */
	accessCodeMoveType := models.SelectedMoveTypePPM
	testdatagen.MakeAccessCode(db, testdatagen.Assertions{
		AccessCode: models.AccessCode{
			Code:     "X3FQJK",
			MoveType: &accessCodeMoveType,
		},
	})

	/*
	 * Service member with a ppm ready to request payment
	 */
	email = "ppm@requestingpayment.newflow"
	uuidStr = "745e0eba-4028-4c78-a262-818b00802748"
	testdatagen.MakeUser(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString(uuidStr)),
			LoginGovEmail: email,
		},
	})
	ppm6 := testdatagen.MakePPM(db, testdatagen.Assertions{
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("1404fdcf-7a54-4b83-862d-7d1c7ba36ad7"),
			UserID:        uuid.FromStringOrNil(uuidStr),
			FirstName:     models.StringPointer("PPM"),
			LastName:      models.StringPointer("RequestingPayNewFlow"),
			Edipi:         models.StringPointer("6737033007"),
			PersonalEmail: models.StringPointer(email),
		},
		// These values should be populated for an approved move
		Order: models.Order{
			OrdersNumber:        models.StringPointer("62149"),
			OrdersTypeDetail:    &typeDetail,
			DepartmentIndicator: models.StringPointer("AIR_FORCE"),
			TAC:                 models.StringPointer("99"),
		},
		Move: models.Move{
			ID:      uuid.FromStringOrNil("f9f10492-587e-43b3-af2a-9f67d2ac8757"),
			Locator: "RQPAY2",
		},
		PersonallyProcuredMove: models.PersonallyProcuredMove{
			OriginalMoveDate: &pastTime,
		},
		Uploader: loader,
	})
	ppm6.Move.Submit(time.Now())
	ppm6.Move.Approve()
	ppm6.Move.PersonallyProcuredMoves[0].Submit(time.Now())
	ppm6.Move.PersonallyProcuredMoves[0].Approve(time.Now())
	models.SaveMoveDependencies(db, &ppm6.Move)

	/*
	 * Service member with a ppm ready to request payment
	 */
	email = "ppm@continue.requestingpayment"
	uuidStr = "4ebc03b7-c801-4c0d-806c-a95aed242102"
	testdatagen.MakeUser(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString(uuidStr)),
			LoginGovEmail: email,
		},
	})
	ppm7 := testdatagen.MakePPM(db, testdatagen.Assertions{
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("0cfb9fc6-82dd-404b-aa39-4deb6dba6c66"),
			UserID:        uuid.FromStringOrNil(uuidStr),
			FirstName:     models.StringPointer("PPM"),
			LastName:      models.StringPointer("ContinueRequesting"),
			Edipi:         models.StringPointer("6737033007"),
			PersonalEmail: models.StringPointer(email),
		},
		// These values should be populated for an approved move
		Order: models.Order{
			OrdersNumber:        models.StringPointer("62149"),
			OrdersTypeDetail:    &typeDetail,
			DepartmentIndicator: models.StringPointer("AIR_FORCE"),
			TAC:                 models.StringPointer("99"),
		},
		Move: models.Move{
			ID:      uuid.FromStringOrNil("0581253d-0539-4a93-b1b6-ea4ad384f0c5"),
			Locator: "RQPAY3",
		},
		PersonallyProcuredMove: models.PersonallyProcuredMove{
			OriginalMoveDate: &pastTime,
		},
		Uploader: loader,
	})
	ppm7.Move.Submit(time.Now())
	ppm7.Move.Approve()
	ppm7.Move.PersonallyProcuredMoves[0].Submit(time.Now())
	ppm7.Move.PersonallyProcuredMoves[0].Approve(time.Now())
	models.SaveMoveDependencies(db, &ppm7.Move)
}

// MakeHhgWithPpm creates an HHG user who has added a PPM
func MakeHhgWithPpm(db *pop.Connection, tspUser models.TspUser, loader *uploader.Uploader) {
	email := "hhg@with.ppm"
	userID := uuid.Must(uuid.FromString("4440f0cf-63e3-4722-b5aa-ba46f8f7ac64"))
	smID := uuid.FromStringOrNil("8232beef-1cdf-4117-9db2-aad548f54430")
	moveID := uuid.FromStringOrNil("5555ef45-8145-487b-9b59-0e30d0d465fa")

	testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            userID,
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            smID,
			FirstName:     models.StringPointer("HHGPPM"),
			LastName:      models.StringPointer("Submitted"),
			Edipi:         models.StringPointer("4224567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Order: models.Order{
			IssueDate: time.Date(testdatagen.TestYear, time.May, 20, 0, 0, 0, 0, time.UTC),
		},
		Move: models.Move{
			ID:               moveID,
			Locator:          "HHGPPM",
			SelectedMoveType: &selectedMoveTypeHHGPPM,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("73335a23-2830-4de0-bb6a-7698a25865cb"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:             models.ShipmentStatusAWARDED,
			HasDeliveryAddress: true,
			GBLNumber:          models.StringPointer("LKNQ7123456"),
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
		},
	})

	ppm := testdatagen.MakePPM(db, testdatagen.Assertions{
		ServiceMember: models.ServiceMember{
			ID: smID,
		},
		Move: models.Move{
			ID: moveID,
		},
		PersonallyProcuredMove: models.PersonallyProcuredMove{
			OriginalMoveDate: &nextValidMoveDate,
			MoveID:           moveID,
		},
		Uploader: loader,
	})
	ppm.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &ppm.Move)

	email2 := "hhgwithppm@approve.shipment"
	userID2 := uuid.Must(uuid.FromString("144f63c5-676f-4c36-a849-5588bd60c33f"))
	moveID2 := uuid.FromStringOrNil("27266e89-df79-4469-8843-05b45741a818")
	smID2 := uuid.FromStringOrNil("f753c28f-5da4-4924-9955-dba90b1b3011")
	ordersTypeDetail := internalmessages.OrdersTypeDetailPCSTDY

	testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            userID2,
			LoginGovEmail: email2,
		},
		ServiceMember: models.ServiceMember{
			ID:            smID2,
			FirstName:     models.StringPointer("HHGPPM"),
			LastName:      models.StringPointer("ApproveShipment"),
			Edipi:         models.StringPointer("4224567890"),
			PersonalEmail: models.StringPointer(email2),
		},
		Order: models.Order{
			IssueDate:        time.Date(testdatagen.TestYear, time.May, 20, 0, 0, 0, 0, time.UTC),
			OrdersTypeDetail: &ordersTypeDetail,
			SAC:              models.StringPointer("sac"),
		},
		Move: models.Move{
			ID:               moveID2,
			Locator:          "COMBO2",
			SelectedMoveType: &selectedMoveTypeHHGPPM,
			Status:           models.MoveStatusAPPROVED,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("ceb40c74-41a4-48cd-a53b-f7f5a81c7ebc"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:             models.ShipmentStatusACCEPTED,
			HasDeliveryAddress: true,
			GBLNumber:          models.StringPointer("LKNQ7123456"),
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
		},
	})

	ppm2 := testdatagen.MakePPM(db, testdatagen.Assertions{
		ServiceMember: models.ServiceMember{
			ID: smID2,
		},
		Move: models.Move{
			ID: moveID2,
		},
		PersonallyProcuredMove: models.PersonallyProcuredMove{
			OriginalMoveDate: &nextValidMoveDate,
			MoveID:           moveID2,
		},
		Uploader: loader,
	})
	ppm2.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &ppm2.Move)

	email3 := "delivered@hhg.ppm"
	userID3 := uuid.Must(uuid.FromString("70ad85ce-bb8b-4def-b138-344d7438efe2"))
	moveID3 := uuid.FromStringOrNil("fbde058b-fed7-4612-8f48-5cf6d17c8ded")
	smID3 := uuid.FromStringOrNil("f443ed0c-576d-41dd-9815-436c714cfd79")
	weightEstimate := unit.Pound(5000)
	sourceOffice := testdatagen.MakeTransportationOffice(db, testdatagen.Assertions{
		TransportationOffice: models.TransportationOffice{
			Gbloc: "ABCD",
		},
	})
	destOffice := testdatagen.MakeTransportationOffice(db, testdatagen.Assertions{
		TransportationOffice: models.TransportationOffice{
			Gbloc: "QRED",
		},
	})
	testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            userID3,
			LoginGovEmail: email3,
		},
		ServiceMember: models.ServiceMember{
			ID:            smID3,
			FirstName:     models.StringPointer("HHGPPM"),
			LastName:      models.StringPointer("Delivered"),
			Edipi:         models.StringPointer("4224567890"),
			PersonalEmail: models.StringPointer(email3),
		},
		Order: models.Order{
			IssueDate:        time.Date(testdatagen.TestYear, time.May, 20, 0, 0, 0, 0, time.UTC),
			OrdersTypeDetail: &ordersTypeDetail,
		},
		Move: models.Move{
			ID:               moveID3,
			Locator:          "COMBO3",
			SelectedMoveType: &selectedMoveTypeHHGPPM,
			Status:           models.MoveStatusAPPROVED,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("efe2d407-af23-4838-aba8-5bc6dd6696ba"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:             models.ShipmentStatusDELIVERED,
			HasDeliveryAddress: true,
			GBLNumber:          models.StringPointer("LKNQ7123456"),

			ActualPickupDate:            &nextValidMoveDateMinusFive,
			PmSurveyMethod:              "PHONE",
			PmSurveyPlannedPackDate:     &nextValidMoveDatePlusOne,
			PmSurveyPlannedPickupDate:   &nextValidMoveDatePlusFive,
			PmSurveyConductedDate:       &nextValidMoveDatePlusOne,
			PmSurveyCompletedAt:         &nextValidMoveDatePlusOne,
			PmSurveyPlannedDeliveryDate: &nextValidMoveDatePlusTen,
			PmSurveyWeightEstimate:      &weightEstimate,
			SourceGBLOC:                 &sourceOffice.Gbloc,
			DestinationGBLOC:            &destOffice.Gbloc,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
		},
	})

	ppm3 := testdatagen.MakePPM(db, testdatagen.Assertions{
		ServiceMember: models.ServiceMember{
			ID: smID3,
		},
		Move: models.Move{
			ID: moveID3,
		},
		PersonallyProcuredMove: models.PersonallyProcuredMove{
			OriginalMoveDate: &nextValidMoveDate,
			MoveID:           moveID3,
		},
		Uploader: loader,
	})
	ppm3.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &ppm3.Move)
}

// MakeHhgFromAwardedToAcceptedGBLReady creates a scenario for an approved shipment ready for GBL generation
func MakeHhgFromAwardedToAcceptedGBLReady(db *pop.Connection, tspUser models.TspUser) models.Shipment {
	/*
	 * Service member with uploaded orders and an approved shipment to be accepted, able to generate GBL
	 */
	email := "hhg@govbilloflading.ready"
	ordersTypeDetail := internalmessages.OrdersTypeDetailHHGPERMITTED
	weightEstimate := unit.Pound(5000)
	sourceOffice := testdatagen.MakeTransportationOffice(db, testdatagen.Assertions{
		TransportationOffice: models.TransportationOffice{
			Gbloc: "ABCD",
		},
	})
	destOffice := testdatagen.MakeTransportationOffice(db, testdatagen.Assertions{
		TransportationOffice: models.TransportationOffice{
			Gbloc: "QRED",
		},
	})
	GBLNumber := destOffice.Gbloc + "001234"
	offer9 := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("658f3a78-b3a9-47f4-a820-af673103d62d")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("658f3a78-b3a9-47f4-a820-af673103d62d"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("ReadyForGBL"),
			Edipi:         models.StringPointer("4444567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Order: models.Order{
			DepartmentIndicator: models.StringPointer("AIR_FORCE"),
			TAC:                 models.StringPointer("NTA4"),
			SAC:                 models.StringPointer("1234567890 9876543210"),
			OrdersTypeDetail:    &ordersTypeDetail,
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("05a58b2e-07da-4b41-b4f8-d18ab68dddd5"),
			Locator:          "GBLGBL",
			SelectedMoveType: &selectedMoveTypeHHG,
			Status:           models.MoveStatusAPPROVED,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("b15fdc2b-52cd-4b3e-91ba-a36d6ab94a16"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},

		Shipment: models.Shipment{
			ID:                          uuid.FromStringOrNil("a4013cee-aa0a-41a3-b5f5-b9eed0758e1d 0xc42022c070"),
			Status:                      models.ShipmentStatusAPPROVED,
			PmSurveyMethod:              "PHONE",
			PmSurveyPlannedPackDate:     &nextValidMoveDatePlusOne,
			PmSurveyPlannedPickupDate:   &nextValidMoveDatePlusFive,
			PmSurveyCompletedAt:         &nextValidMoveDatePlusOne,
			PmSurveyPlannedDeliveryDate: &nextValidMoveDatePlusTen,
			PmSurveyWeightEstimate:      &weightEstimate,
			SourceGBLOC:                 &sourceOffice.Gbloc,
			DestinationGBLOC:            &destOffice.Gbloc,
			GBLNumber:                   &GBLNumber,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			TransportationServiceProvider:   tspUser.TransportationServiceProvider,
			Accepted:                        models.BoolPointer(true),
		},
	})

	testdatagen.MakeServiceAgent(db, testdatagen.Assertions{
		ServiceAgent: models.ServiceAgent{
			Shipment:   &offer9.Shipment,
			ShipmentID: offer9.ShipmentID,
		},
	})

	hhg2 := offer9.Shipment
	hhg2.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg2.Move)
	return offer9.Shipment
}

// MakeHhgWithGBL creates a scenario for an approved shipment with a GBL generated
func MakeHhgWithGBL(db *pop.Connection, tspUser models.TspUser, logger Logger, storer *storage.Memory) models.Shipment {
	/*
	 * Service member with uploaded orders and an approved shipment to be accepted, able to generate GBL
	 */
	email := "hhg@gov_bill_of_lading.created"
	ordersTypeDetail := internalmessages.OrdersTypeDetailHHGPERMITTED

	weightEstimate := unit.Pound(5000)
	sourceOffice := testdatagen.MakeTransportationOffice(db, testdatagen.Assertions{
		TransportationOffice: models.TransportationOffice{
			Gbloc: "ABCD",
		},
	})
	destOffice := testdatagen.MakeTransportationOffice(db, testdatagen.Assertions{
		TransportationOffice: models.TransportationOffice{
			Gbloc: "QRED",
		},
	})
	offer := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("b7dccea1-d052-4a66-aed9-2fdacf461023")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("8a1a86c7-78d6-4897-806e-0e4c5546fdec"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("HasGBL"),
			Edipi:         models.StringPointer("4444567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Order: models.Order{
			DepartmentIndicator: models.StringPointer("17"),
			TAC:                 models.StringPointer("NTA4"),
			SAC:                 models.StringPointer("1234567890 9876543210"),
			OrdersTypeDetail:    &ordersTypeDetail,
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("6eee3663-1973-40c5-b49e-e70e9325b895"),
			Locator:          "CONGBL",
			SelectedMoveType: &selectedMoveTypeHHG,
			Status:           models.MoveStatusAPPROVED,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("87fcebf6-63b8-40cb-bc40-b553f5b91b9c"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			ID:     uuid.FromStringOrNil("0851706a-997f-46fb-84e4-2525a444ade0"),
			Status: models.ShipmentStatusAPPROVED,

			PmSurveyMethod:              "PHONE",
			PmSurveyPlannedPackDate:     &nextValidMoveDatePlusOne,
			PmSurveyPlannedPickupDate:   &nextValidMoveDatePlusFive,
			PmSurveyCompletedAt:         &nextValidMoveDatePlusOne,
			PmSurveyPlannedDeliveryDate: &nextValidMoveDatePlusTen,
			PmSurveyWeightEstimate:      &weightEstimate,
			SourceGBLOC:                 &sourceOffice.Gbloc,
			DestinationGBLOC:            &destOffice.Gbloc,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			TransportationServiceProvider:   tspUser.TransportationServiceProvider,
			Accepted:                        models.BoolPointer(true),
		},
	})

	testdatagen.MakeServiceAgent(db, testdatagen.Assertions{
		ServiceAgent: models.ServiceAgent{
			Shipment:   &offer.Shipment,
			ShipmentID: offer.ShipmentID,
		},
	})

	hhg := offer.Shipment
	hhgID := offer.ShipmentID
	hhg.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg.Move)

	// Create PDF for GBL
	gbl, _ := models.FetchGovBillOfLadingFormValues(db, hhgID)
	formLayout := paperwork.Form1203Layout

	// Read in bytes from Asset pkg
	data, _ := assets.Asset(formLayout.TemplateImagePath)
	f := bytes.NewReader(data)

	formFiller := paperwork.NewFormFiller()
	formFiller.AppendPage(f, formLayout.FieldsLayout, gbl)

	// Write to a temporary file system
	aFile, _ := storer.TempFileSystem().Create(gbl.GBLNumber1)
	formFiller.Output(aFile)

	uploader := uploaderpkg.NewUploader(db, logger, storer)
	upload, _, _ := uploader.CreateUpload(*tspUser.UserID, &aFile, uploaderpkg.AllowedTypesPDF)
	uploads := []models.Upload{*upload}

	// Create GBL move document associated to the shipment
	hhg.Move.CreateMoveDocument(db,
		uploads,
		&hhgID,
		models.MoveDocumentTypeGOVBILLOFLADING,
		string("Government Bill Of Lading"),
		swag.String(""),
		models.SelectedMoveTypeHHG,
	)

	return offer.Shipment
}

type placeInSITParams struct {
	SITID           uuid.UUID
	ShipmentID      uuid.UUID
	Shipment        models.Shipment
	ActualStartDate time.Time
}

func sitPlaceInSIT(db *pop.Connection, params placeInSITParams, tspUserSession auth.Session) {
	// Transition SIT to InSIT
	payload := apimessages.StorageInTransitInSitPayload{
		ActualStartDate: *handlers.FmtDate(params.ActualStartDate),
	}

	inSITPlacer := storageintransit.NewStorageInTransitInSITPlacer(db)
	_, verrs, err := inSITPlacer.PlaceIntoSITStorageInTransit(payload, params.Shipment.ID, &tspUserSession, params.SITID)
	if verrs.HasAny() || err != nil {
		fmt.Println(verrs.String())
		log.Panic(err)
	}
}

type hhgReadyToInvoiceParams struct {
	TspUser                models.TspUser
	Logger                 Logger
	Storer                 *storage.Memory
	Email                  string
	NetWeight              unit.Pound
	WeightEstimate         unit.Pound
	SourceGBLOC            string
	DestGBLOC              string
	GBLNumber              string
	ServiceMemberFirstName string
	ServiceMemberLastName  string
	EDIPI                  string
	Locator                string
	PlannerDistance        int
	SITs                   models.StorageInTransits
}

// makeHhgReadyToInvoiceWithSIT creates a shipment that is delivered and invoice ready with the option
// to add a SIT while shipment is in-transit
func makeHhgReadyToInvoiceWithSIT(db *pop.Connection, params hhgReadyToInvoiceParams) models.Shipment {
	/*
	 * Service member with uploaded orders and a delivered shipment, able to generate GBL
	 */
	email := params.Email
	netWeight := unit.Pound(params.NetWeight)
	weightEstimate := unit.Pound(params.WeightEstimate)
	sourceOffice := testdatagen.MakeTransportationOffice(db, testdatagen.Assertions{
		TransportationOffice: models.TransportationOffice{
			Gbloc: params.SourceGBLOC,
		},
	})
	destOffice := testdatagen.MakeTransportationOffice(db, testdatagen.Assertions{
		TransportationOffice: models.TransportationOffice{
			Gbloc: params.DestGBLOC,
		},
	})
	GBLNumber := destOffice.Gbloc + params.GBLNumber

	offer := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.NewV4()),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.Must(uuid.NewV4()),
			FirstName:     models.StringPointer(params.ServiceMemberFirstName),
			LastName:      models.StringPointer(params.ServiceMemberLastName),
			Edipi:         models.StringPointer(params.EDIPI),
			PersonalEmail: models.StringPointer(email),
		},
		Order: models.Order{
			DepartmentIndicator: models.StringPointer("17"),
			TAC:                 models.StringPointer("NTA4"),
			SAC:                 models.StringPointer("1234567890 9876543210"),
		},
		Move: models.Move{
			ID:               uuid.Must(uuid.NewV4()),
			Locator:          params.Locator,
			SelectedMoveType: &selectedMoveTypeHHG,
			Status:           models.MoveStatusAPPROVED,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.Must(uuid.NewV4()),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			ID:                          uuid.Must(uuid.NewV4()),
			Status:                      models.ShipmentStatusINTRANSIT,
			PmSurveyMethod:              "PHONE",
			PmSurveyPlannedPackDate:     &nextValidMoveDateMinusTen,
			PmSurveyPlannedPickupDate:   &nextValidMoveDateMinusFive,
			PmSurveyPlannedDeliveryDate: &nextValidMoveDateMinusOne,
			NetWeight:                   &netWeight,
			ActualPickupDate:            &nextValidMoveDateMinusFive,
			OriginalDeliveryDate:        &nextValidMoveDateMinusOne,
			PmSurveyWeightEstimate:      &weightEstimate,
			SourceGBLOC:                 &sourceOffice.Gbloc,
			DestinationGBLOC:            &destOffice.Gbloc,
			GBLNumber:                   &GBLNumber,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: params.TspUser.TransportationServiceProviderID,
			TransportationServiceProvider:   params.TspUser.TransportationServiceProvider,
			Accepted:                        models.BoolPointer(true),
		},
	})

	//
	// Add SIT before delivering shipment -- this seems like the typical flow a shipment will go through
	//
	for _, sit := range params.SITs {
		sit.ShipmentID = offer.Shipment.ID
		sit.Shipment = offer.Shipment
		testdatagen.MakeStorageInTransit(db, testdatagen.Assertions{
			StorageInTransit: sit,
		})
	}

	//
	// Get Planner and Deliver shipment
	//
	engine := rateengine.NewRateEngine(db, params.Logger)

	verrs, err := shipmentservice.NewShipmentDeliverAndPricer(
		db,
		engine,
		route.NewTestingPlanner(1044),
	).DeliverAndPriceShipment(nextValidMoveDateMinusOne, &offer.Shipment)

	if verrs.HasAny() || err != nil {
		fmt.Println(verrs.String())
		log.Panic(err)
	}

	testdatagen.MakeServiceAgent(db, testdatagen.Assertions{
		ServiceAgent: models.ServiceAgent{
			Shipment:   &offer.Shipment,
			ShipmentID: offer.ShipmentID,
		},
	})

	hhg := offer.Shipment
	hhgID := offer.ShipmentID
	hhg.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg.Move)

	// Create PDF for GBL
	gbl, _ := models.FetchGovBillOfLadingFormValues(db, hhgID)
	formLayout := paperwork.Form1203Layout

	// Read in bytes from Asset pkg
	data, _ := assets.Asset(formLayout.TemplateImagePath)
	f := bytes.NewReader(data)

	formFiller := paperwork.NewFormFiller()
	formFiller.AppendPage(f, formLayout.FieldsLayout, gbl)

	// Write to a temporary file system
	aFile, _ := params.Storer.TempFileSystem().Create(gbl.GBLNumber1)
	formFiller.Output(aFile)

	uploader := uploaderpkg.NewUploader(db, params.Logger, params.Storer)
	upload, _, _ := uploader.CreateUpload(*params.TspUser.UserID, &aFile, uploaderpkg.AllowedTypesPDF)
	uploads := []models.Upload{*upload}

	// Create GBL move document associated to the shipment
	hhg.Move.CreateMoveDocument(db,
		uploads,
		&hhgID,
		models.MoveDocumentTypeGOVBILLOFLADING,
		string("Government Bill Of Lading"),
		swag.String(""),
		selectedMoveTypeHHG,
	)

	return offer.Shipment
}

func makeHhgReadyToInvoice(db *pop.Connection, tspUser models.TspUser, logger Logger, storer *storage.Memory) models.Shipment {
	/*
	 * Service member with uploaded orders and a delivered shipment, able to generate GBL
	 */
	email := "hhg@ready_to.invoice"
	netWeight := unit.Pound(3000)
	weightEstimate := unit.Pound(5000)
	sourceOffice := testdatagen.MakeTransportationOffice(db, testdatagen.Assertions{
		TransportationOffice: models.TransportationOffice{
			Gbloc: "ABCD",
		},
	})
	destOffice := testdatagen.MakeTransportationOffice(db, testdatagen.Assertions{
		TransportationOffice: models.TransportationOffice{
			Gbloc: "QRED",
		},
	})
	GBLNumber := destOffice.Gbloc + "001234"

	offer := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("c396388a-de1a-11e8-9f32-f2801f1b9fd1")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("dc281350-de1a-11e8-9f32-f2801f1b9fd1"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("ReadyToInvoice"),
			Edipi:         models.StringPointer("4444567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Order: models.Order{
			DepartmentIndicator: models.StringPointer("17"),
			TAC:                 models.StringPointer("NTA4"),
			SAC:                 models.StringPointer("1234567890 9876543210"),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("fb4105cf-f5a5-43be-845e-d59fdb34f31c"),
			Locator:          "DOOB",
			SelectedMoveType: &selectedMoveTypeHHG,
			Status:           models.MoveStatusAPPROVED,
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("5a60e1f7-63ae-45b1-b574-5aa53b70c9d6"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			ID:                          uuid.FromStringOrNil("67a3cbe7-4ae3-4f6a-9f9a-4f312e7458b9"),
			Status:                      models.ShipmentStatusINTRANSIT,
			PmSurveyMethod:              "PHONE",
			PmSurveyPlannedPackDate:     &nextValidMoveDateMinusTen,
			PmSurveyPlannedPickupDate:   &nextValidMoveDateMinusFive,
			PmSurveyPlannedDeliveryDate: &nextValidMoveDateMinusOne,
			NetWeight:                   &netWeight,
			ActualPickupDate:            &nextValidMoveDateMinusFive,
			OriginalDeliveryDate:        &nextValidMoveDateMinusOne,
			PmSurveyWeightEstimate:      &weightEstimate,
			SourceGBLOC:                 &sourceOffice.Gbloc,
			DestinationGBLOC:            &destOffice.Gbloc,
			GBLNumber:                   &GBLNumber,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
			TransportationServiceProvider:   tspUser.TransportationServiceProvider,
			Accepted:                        models.BoolPointer(true),
		},
	})

	testdatagen.MakeStorageInTransit(db, testdatagen.Assertions{
		StorageInTransit: models.StorageInTransit{
			ShipmentID: offer.ShipmentID,
			Shipment:   offer.Shipment,
			Status:     models.StorageInTransitStatusINSIT,
			Location:   models.StorageInTransitLocationDESTINATION,
		},
	})

	engine := rateengine.NewRateEngine(db, logger)
	verrs, err := shipmentservice.NewShipmentDeliverAndPricer(
		db,
		engine,
		route.NewTestingPlanner(1044),
	).DeliverAndPriceShipment(nextValidMoveDateMinusOne, &offer.Shipment)

	if verrs.HasAny() || err != nil {
		fmt.Println(verrs.String())
		log.Panic(err)
	}

	testdatagen.MakeServiceAgent(db, testdatagen.Assertions{
		ServiceAgent: models.ServiceAgent{
			Shipment:   &offer.Shipment,
			ShipmentID: offer.ShipmentID,
		},
	})

	hhg := offer.Shipment
	hhgID := offer.ShipmentID
	hhg.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg.Move)

	// Create PDF for GBL
	gbl, _ := models.FetchGovBillOfLadingFormValues(db, hhgID)
	formLayout := paperwork.Form1203Layout

	// Read in bytes from Asset pkg
	data, _ := assets.Asset(formLayout.TemplateImagePath)
	f := bytes.NewReader(data)

	formFiller := paperwork.NewFormFiller()
	formFiller.AppendPage(f, formLayout.FieldsLayout, gbl)

	// Write to a temporary file system
	aFile, _ := storer.TempFileSystem().Create(gbl.GBLNumber1)
	formFiller.Output(aFile)

	uploader := uploaderpkg.NewUploader(db, logger, storer)
	upload, _, _ := uploader.CreateUpload(*tspUser.UserID, &aFile, uploaderpkg.AllowedTypesPDF)
	uploads := []models.Upload{*upload}

	// Create GBL move document associated to the shipment
	hhg.Move.CreateMoveDocument(db,
		uploads,
		&hhgID,
		models.MoveDocumentTypeGOVBILLOFLADING,
		string("Government Bill Of Lading"),
		swag.String(""),
		selectedMoveTypeHHG,
	)

	return offer.Shipment
}

func makeHhgShipment(db *pop.Connection, tspUser models.TspUser) models.Shipment {
	/*
	 * Service member with uploaded orders and an approved shipment with show = false
	 */
	email := "hhg@dates.panel"

	offer := testdatagen.MakeShipmentOffer(db, testdatagen.Assertions{
		User: models.User{
			ID:            uuid.Must(uuid.FromString("1d4aad25-b31b-486f-bfae-09d77e729044")),
			LoginGovEmail: email,
		},
		ServiceMember: models.ServiceMember{
			ID:            uuid.FromStringOrNil("c61cae62-b086-411e-a9f9-e75319037a28"),
			FirstName:     models.StringPointer("HHG"),
			LastName:      models.StringPointer("Submitted"),
			Edipi:         models.StringPointer("4444567890"),
			PersonalEmail: models.StringPointer(email),
		},
		Move: models.Move{
			ID:               uuid.FromStringOrNil("360fe5ef-b9e5-4183-9766-3efaeb8113d1"),
			Locator:          "NOSHOW",
			SelectedMoveType: &selectedMoveTypeHHG,
			Show:             swag.Bool(false),
		},
		TrafficDistributionList: models.TrafficDistributionList{
			ID:                uuid.FromStringOrNil("2bfb3da2-481e-42de-ae8f-fb10a2b030ca"),
			SourceRateArea:    "US62",
			DestinationRegion: "11",
			CodeOfService:     "D",
		},
		Shipment: models.Shipment{
			Status:             models.ShipmentStatusAWARDED,
			ActualDeliveryDate: nil,
		},
		ShipmentOffer: models.ShipmentOffer{
			TransportationServiceProviderID: tspUser.TransportationServiceProviderID,
		},
	})

	hhg := offer.Shipment
	hhg.Move.Submit(time.Now())
	models.SaveMoveDependencies(db, &hhg.Move)

	return offer.Shipment
}
