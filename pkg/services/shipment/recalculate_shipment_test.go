package shipment

import (
	"go.uber.org/zap"

	"github.com/transcom/mymove/pkg/models"
	"github.com/transcom/mymove/pkg/rateengine"
	"github.com/transcom/mymove/pkg/route"
	"github.com/transcom/mymove/pkg/services/invoice"
	"github.com/transcom/mymove/pkg/testdatagen"
	"github.com/transcom/mymove/pkg/unit"
)

func (suite *ShipmentServiceSuite) helperDeliverAndPriceShipment() *models.Shipment {
	numTspUsers := 1
	numShipments := 1
	numShipmentOfferSplit := []int{1}
	status := []models.ShipmentStatus{models.ShipmentStatusINTRANSIT}
	_, shipments, _, err := testdatagen.CreateShipmentOfferData(suite.DB(), numTspUsers, numShipments, numShipmentOfferSplit, status, models.SelectedMoveTypeHHG)
	suite.FatalNoError(err)

	shipment := shipments[0]

	// And an unpriced, approved pre-approval
	testdatagen.MakeCompleteShipmentLineItem(suite.DB(), testdatagen.Assertions{
		ShipmentLineItem: models.ShipmentLineItem{
			Shipment:   shipment,
			ShipmentID: shipment.ID,
			Status:     models.ShipmentLineItemStatusAPPROVED,
		},
		Tariff400ngItem: models.Tariff400ngItem{
			RequiresPreApproval: true,
		},
	})

	// Add storage in transit
	authorizedStartDate := shipment.ActualPickupDate
	actualStartDate := authorizedStartDate.Add(testdatagen.OneDay)
	sit := testdatagen.MakeStorageInTransit(suite.DB(), testdatagen.Assertions{
		StorageInTransit: models.StorageInTransit{
			ShipmentID:          shipment.ID,
			Shipment:            shipment,
			EstimatedStartDate:  *authorizedStartDate,
			AuthorizedStartDate: authorizedStartDate,
			ActualStartDate:     &actualStartDate,
			Status:              models.StorageInTransitStatusINSIT,
			Location:            models.StorageInTransitLocationDESTINATION,
		},
	})

	shipment.StorageInTransits = models.StorageInTransits{sit}

	// Make sure there's a FuelEIADieselPrice
	assertions := testdatagen.Assertions{}
	assertions.FuelEIADieselPrice.BaselineRate = 6
	testdatagen.MakeFuelEIADieselPrices(suite.DB(), assertions)

	deliveryDate := testdatagen.DateInsidePerformancePeriod
	engine := rateengine.NewRateEngine(suite.DB(), suite.logger)
	verrs, err := NewShipmentDeliverAndPricer(
		suite.DB(),
		engine,
		route.NewTestingPlanner(1044),
	).DeliverAndPriceShipment(deliveryDate, &shipment)
	suite.FatalNoError(err)
	suite.FatalFalse(verrs.HasAny())

	suite.Equal(shipment.Status, models.ShipmentStatusDELIVERED)

	fetchedLineItems, err := models.FetchLineItemsByShipmentID(suite.DB(), &shipment.ID)
	suite.FatalNoError(err)
	// All items should be priced
	for _, item := range fetchedLineItems {
		suite.NotNil(item.AmountCents, item.Tariff400ngItem.Code)
	}

	return &shipment
}

func (suite *ShipmentServiceSuite) TestRecalculateShipmentCall() {

	shipment := suite.helperDeliverAndPriceShipment()
	shipmentID := shipment.ID

	fetchedLineItems, err := models.FetchLineItemsByShipmentID(suite.DB(), &shipmentID)
	suite.FatalNoError(err)
	// All items should be priced
	for _, item := range fetchedLineItems {
		suite.NotNil(item.AmountCents, item.Tariff400ngItem.Code)
	}

	// Verify that all base shipment line items are present
	allPresent := ProcessRecalculateShipment{}.hasAllBaseLineItems(fetchedLineItems)
	suite.Equal(true, allPresent)

	// Remove 1 base shipment line time
	// Removing Fuel Surcharge
	var fuelSurcharge models.ShipmentLineItem
	removedFuelSurcharge := false
	for _, item := range fetchedLineItems {
		if item.Tariff400ngItem.Code == "16A" {
			fuelSurcharge = item
			destroyErr := suite.DB().Destroy(&fuelSurcharge)
			if destroyErr != nil {
				suite.logger.Fatal("Error Removing Fuel Surcharge", zap.Error(destroyErr))
			}
			removedFuelSurcharge = true
		}
	}
	suite.Equal(true, removedFuelSurcharge)

	zeroCents := unit.Cents(0)
	zeroMillicents := unit.Millicents(0)

	// Set price of 1 base shipment line item to zero
	updatedUnpack := false
	for _, item := range fetchedLineItems {
		if item.Tariff400ngItem.Code == "105C" {
			item.AmountCents = &zeroCents
			item.AppliedRate = &zeroMillicents
			updatedUnpack = true
			suite.MustSave(&item)
			break
		}
	}
	suite.Equal(true, updatedUnpack)

	// Find 210 line item
	updated210 := false
	var item210 models.ShipmentLineItem
	for _, item := range fetchedLineItems {
		if item.Tariff400ngItem.Code == "210A" || item.Tariff400ngItem.Code == "210B" || item.Tariff400ngItem.Code == "210C" {
			item210 = item
			item.AmountCents = &zeroCents
			item.AppliedRate = &zeroMillicents
			updated210 = true
			suite.MustSave(&item)
			break
		}
	}
	suite.Equal(true, updated210)

	// Fetch shipment line items after saves
	fetchedLineItems, err = models.FetchLineItemsByShipmentID(suite.DB(), &shipmentID)
	suite.FatalNoError(err)

	// Verify base shipment line item is zero and 210 item is zero
	for _, item := range fetchedLineItems {
		if item.Tariff400ngItem.Code == "4A" {
			suite.Equal(zeroCents, *item.AmountCents)
		}
		if item.Tariff400ngItem.Code == item210.Tariff400ngItem.Code {
			suite.Equal(zeroCents, *item.AmountCents)
		}
	}

	shipment2, err := invoice.FetchShipmentForInvoice{DB: suite.DB()}.Call(shipmentID)
	if err != nil {
		suite.logger.Error("Error fetching Shipment for re-pricing line items for shipment", zap.Error(err))
	}
	shipment = &shipment2

	// Verify all base shipment line items are not present
	allPresent = ProcessRecalculateShipment{}.hasAllBaseLineItems(fetchedLineItems)
	suite.Equal(false, allPresent)

	// Re-calculate the Shipment!
	planner := route.NewTestingPlanner(1100)
	engine := rateengine.NewRateEngine(suite.DB(), suite.logger)
	verrs, err := RecalculateShipment{
		DB:      suite.DB(),
		Logger:  suite.logger,
		Engine:  engine,
		Planner: planner,
	}.Call(shipment)
	suite.Equal(false, verrs.HasAny())
	suite.Nil(err, "Failed to recalculate shipment")

	// Fetch shipment line items after recalculation
	fetchedLineItems, err = models.FetchLineItemsByShipmentID(suite.DB(), &shipmentID)
	suite.FatalNoError(err)

	// Verify all base shipment line items are present
	allPresent = ProcessRecalculateShipment{}.hasAllBaseLineItems(fetchedLineItems)
	suite.Equal(true, allPresent)

	// Verify 210 line item is present
	found210 := false
	for _, item := range fetchedLineItems {
		if item.Tariff400ngItem.Code == item210.Tariff400ngItem.Code {
			// Date created should be greater than the old item210
			suite.Equal(true, item.CreatedAt.After(item210.CreatedAt), "210 CreatedAt is updated")
			// Price and Discount should be the same
			suite.Equal(item210.AmountCents, item.AmountCents, "210 Price should be the same")
			suite.Equal(item210.AppliedRate, item.AppliedRate, "210 Discount rate should be the same")
			found210 = true
			break
		}
	}
	suite.Equal(true, found210)

	// All items should be priced
	// Verify base shipment line item is not zero
	// Verify approved accessorial is not zero
	for _, item := range fetchedLineItems {
		suite.NotEqual(zeroCents, *item.AmountCents)
	}
}
