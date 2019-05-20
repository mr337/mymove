package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gobuffalo/pop"
	"github.com/gobuffalo/validate"
	"github.com/gofrs/uuid"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"go.uber.org/zap"

	"github.com/transcom/mymove/pkg/models"
	"github.com/transcom/mymove/pkg/rateengine"
	"github.com/transcom/mymove/pkg/route"
	"github.com/transcom/mymove/pkg/unit"
)

func main() {
	logger, err := zap.NewDevelopment()

	flag := pflag.CommandLine
	config := flag.String("config-dir", "config", "The location of server config files")
	env := flag.String("env", "development", "The environment to run in, which configures the database.")
	//shipmentID := flag.String("shipment", "", "The shipment ID to generate 1203 form for")
	//debug := flag.Bool("debug", false, "show field debug output")
	flag.Parse(os.Args[1:])

	// DB connection
	err = pop.AddLookupPaths(*config)
	if err != nil {
		log.Fatal(err)
	}
	//*env = "prod"
	db, err := pop.Connect(*env)
	if err != nil {
		log.Fatal(err)
	}
	pop.Debug = true

	// For each <tariff400ngItem> at <zip location> using the <submitted date> find the <Tariff400ngItemRate>
	// compute <the price and rate>

	// Find the discounted rate
	// discountRate := shipment.ShipmentOffers[0].TransportationServiceProviderPerformance.LinehaulRate

	//engine := NewRateEngine(suite.DB(), suite.logger)
	//computedPriceAndRate, err := engine.ComputeShipmentLineItemCharge(item)

	// Verify zips are present for zip list

	// TODO: have to explicitly do pricing for SIT separately see it in accessorials.go file

	// To price a shipment line item using the function ComputeShipmentLineItemCharge we need a Shipment
	// with fields :
	// 	   NetWeight
	//     PickupAddress.PostalCode
	// 	   Move.Orders.NewDutyStation.Address.PostalCode
	//     BookDate
	//     ShipmentOffers[0].TransportationServiceProviderPerformance.ID

	netWeight := unit.Pound(3000)

	// make a shipment with the following
	pickupAddress := models.Address{
		PostalCode: "62225",
	}
	destinationAddress := models.Address{
		PostalCode: "36109",
	}

	bookDate := time.Date(2019, time.June, 1, 0, 0, 0, 0, time.UTC)
	requestedPickup := time.Date(2019, time.June, 20, 0, 0, 0, 0, time.UTC)

	/*
		func CreateShipmentOffer(tx *pop.Connection,
			shipmentID uuid.UUID,
			tspID uuid.UUID,
			tsppID uuid.UUID,
			administrativeShipment bool) (*ShipmentOffer, error) {
	*/

	shipment := models.Shipment{
		ID:            uuid.Must(uuid.NewV4()),
		NetWeight:     &netWeight,
		PickupAddress: &pickupAddress,
		Move: models.Move{
			Orders: models.Order{
				NewDutyStation: models.DutyStation{
					Address: destinationAddress,
				},
			},
		},
		BookDate:            &bookDate,
		RequestedPickupDate: &requestedPickup,
	}

	tdl, err := determineTrafficDistributionList(db, shipment.PickupAddress.PostalCode, shipment.Move.Orders.NewDutyStation.Address.PostalCode)
	if err != nil || tdl == nil {
		log.Print("Failed to find TDL")
		log.Fatal(tdl)
	}
	log.Print(tdl)
	tspp, err := getATSPP(db, tdl.ID, *shipment.BookDate, *shipment.RequestedPickupDate)
	if err != nil {
		log.Print("Failed to find TSPP with the following TDL")
		if tdl != nil {
			log.Printf("%v\n", tdl)
		}
	}
	log.Print(tspp)
	shipment.TrafficDistributionList = tdl
	shipment.TrafficDistributionListID = &tdl.ID

	accepted := true
	shipmentOffer := models.ShipmentOffer{
		ShipmentID:                               shipment.ID,
		Shipment:                                 shipment,
		TransportationServiceProviderPerformance: tspp,
		TransportationServiceProviderPerformanceID: tspp.ID,
		AdministrativeShipment:                     false,
		Accepted:                                   &accepted, // This is a Tri-state and new offers are always nil until accepted
		RejectionReason:                            nil,
	}
	shipmentOffer.Shipment.ShipmentOffers = append(shipmentOffer.Shipment.ShipmentOffers, shipmentOffer)

	// returns a pointer
	//shipmentOffer := offer

	//shipmentOffers := models.ShipmentOffers{
	//	shipmentOffer,
	//}

	//shipment.ShipmentOffers = shipmentOffers

	// ShipmentLineItems needs the following to get priced:
	//     itemCode := shipmentLineItem.Tariff400ngItem.Code
	//     shipment := shipmentLineItem.Shipment
	//     shipmentLineItem.Location == models.ShipmentLineItemLocationDESTINATION
	//

	engine := rateengine.NewRateEngine(db, logger)

	fmt.Println("====================================================")
	fmt.Printf("Pricing shipment from %s to %s\n", shipment.PickupAddress.PostalCode, shipment.Move.Orders.NewDutyStation.Address.PostalCode)
	fmt.Printf("shipment weight: %d lbs\n", shipment.NetWeight.Int())

	for _, baseLineItemCode := range models.BaseShipmentLineItems {
		fmt.Println("----")
		fmt.Printf("Get tariff400ng_item for code %s\n", baseLineItemCode.Code)
		fetchedItem, err := models.FetchTariff400ngItemByCode(db, baseLineItemCode.Code)
		if err != nil {
			fmt.Printf("%s\n, using db %s\n   ", err.Error(), db.String())
			fmt.Printf("%v\n", db)
			continue
		}

		shipmentLineItem := models.ShipmentLineItem{
			ShipmentID:        shipment.ID,
			Shipment:          shipment,
			Tariff400ngItemID: fetchedItem.ID,
			Tariff400ngItem:   fetchedItem,
			Quantity1:         unit.BaseQuantity(1670),
		}
		location := models.ShipmentLineItemLocationORIGIN
		if isDestinationCode(shipmentLineItem.Tariff400ngItem.Code) {
			location = models.ShipmentLineItemLocationDESTINATION
		}
		shipmentLineItem.Location = location

		hereTestSuite := route.HereFullSuite{}

		var planner route.Planner
		origin := shipment.PickupAddress
		destination := shipment.Move.Orders.NewDutyStation.Address

		distanceCalculation, err := models.NewDistanceCalculation(c.Planner, *origin, destination)
		if err != nil {
			return validate.NewErrors(), errors.Wrap(err, "Error creating DistanceCalculation model")
		}

		// All required relationships should exist at this point.
		daysInSIT := 0
		var sitDiscount unit.DiscountRate
		sitDiscount = 0.0

		lhDiscount := shipmentOffer.TransportationServiceProviderPerformance.LinehaulRate

		// Apply rate engine to shipment
		var shipmentCost CostByShipment
		cost, err := re.ComputeShipment(shipment,
			distanceCalculation,
			daysInSIT, // We don't want any SIT charges
			lhDiscount,
			sitDiscount,
		)

		computedPriceAndRate, err := engine.ComputeShipmentLineItemCharge(shipmentLineItem)
		if err != nil {
			fmt.Printf("ERROR: ComputeShipmentLineItemCharge(): %s\n", err.Error())
		}

		shipmentLineItem.AmountCents = &computedPriceAndRate.Fee

		fmt.Printf("line item %s - %s\n", shipmentLineItem.Tariff400ngItem.Code, baseLineItemCode.Description)
		fmt.Printf("\t\t Price: %s, Discount rate: %s\n", shipmentLineItem.AmountCents.ToDollarString(), computedPriceAndRate.Rate.ToDollarString())
	}

	fmt.Println("Print examples")
	fmt.Printf("Print examples: %v, Print examples: %v \n", db, err)

	//MakeShipmentOffer
	//CreateShipmentOfferData

	// For each base location find the zip/zip5/zip3 then
	//     For each tariff400ngItem at zip
	//         Make a ShipmentLineItem
	//         Get the Tariff400ngItemRate
	//         For each TransportationServiceProviderPerformance
	//             Find the rate using the date(s) (test given date or end of each period)
	//             ComputeShipmentLineItemCharge(shipmentLineItem)
	//             discountRate := shipment.ShipmentOffers[0].TransportationServiceProviderPerformance.LinehaulRate
	//

}

// getATSP returns a TSPP that is valid for the TDL, doesn't do the full evaluation using quality bands or round robin
// review awardqueue.go for the full functionality that goes into picking a TSPP.
func getATSPP(tx *pop.Connection,
	tdlID uuid.UUID,
	bookDate time.Time,
	requestedPickupDate time.Time) (
	models.TransportationServiceProviderPerformance, error) {

	sql := `SELECT
			tspp.*
		FROM
			transportation_service_provider_performances AS tspp
		LEFT JOIN
			transportation_service_providers AS tsp ON
				tspp.transportation_service_provider_id = tsp.id
		WHERE
			tspp.traffic_distribution_list_id = $1
			AND
			$2 BETWEEN tspp.performance_period_start AND tspp.performance_period_end
			AND
			$3 BETWEEN tspp.rate_cycle_start AND tspp.rate_cycle_end
			AND
			tsp.enrolled = true
		ORDER BY
			offer_count ASC,
			best_value_score DESC
		`
	tspp := models.TransportationServiceProviderPerformance{}
	err := tx.RawQuery(sql, tdlID, bookDate, requestedPickupDate).First(&tspp)

	return tspp, err
}

func isDestinationCode(a string) bool {
	destinationCodes := [2]string{"105C", "135B"}
	for _, b := range destinationCodes {
		if b == a {
			return true
		}
	}
	return false
}

// determineTrafficDistributionList (based based off of function shipment.go::models.DetermineTrafficDistributionList)
// attempts to find (or create) the TDL for a shipment.  Since some of
// the fields needed to determine the TDL are optional, this may return a nil TDL in a non-error scenario.
func determineTrafficDistributionList(db *pop.Connection, pickupZip string, destinationZip string) (*models.TrafficDistributionList, error) {
	// To look up a TDL, we need to try to determine the following:
	// 1) source_rate_area: Find using the postal code of the pickup address.
	// 2) destination_region: Find using the postal code of the destination duty station.
	// 3) code_of_service: For now, always assume "D".
	rateArea, err := models.FetchRateAreaForZip5(db, pickupZip)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not fetch rate area for zip %s", pickupZip)
	}

	region, err := models.FetchRegionForZip5(db, destinationZip)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not fetch region for zip %s", destinationZip)
	}

	// Code of service -> hard-coded for now.
	codeOfService := "D"

	// Fetch the TDL (or create it if it doesn't exist already).
	trafficDistributionList, err := models.FetchOrCreateTDL(db, rateArea, region, codeOfService)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not fetch TDL for rateArea=%s, region=%s, codeOfService=%s",
			rateArea, region, codeOfService)
	}

	return &trafficDistributionList, nil
}
