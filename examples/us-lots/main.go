// Copart and IAAI lots: VIN, damage, title and sale date.
//
// market=us holds both auctions and the API has no filter by auction: Source on each record tells
// them apart, so keep the ones you need on your side.
//
//	export APIAUTO_KEY=...
//	go run ./examples/us-lots
package main

import (
	"context"
	"fmt"
	"log"

	apiauto "github.com/apiauto-space/apiauto-go"
)

func main() {
	client, err := apiauto.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	params := &apiauto.CarsParams{Brand: "Toyota", Model: "Camry", YearMin: 2020, Limit: 5}
	for lot, err := range client.IterCars(context.Background(), apiauto.MarketUS, params, 5) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(lot.Source, lot.SourceID, lot.Title, lot.VIN, lot.RawString("damage_main"), lot.RawString("sale_date"))
	}
}
