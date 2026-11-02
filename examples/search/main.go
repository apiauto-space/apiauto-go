// Search Encar (South Korea) by make, model and year.
//
//	export APIAUTO_KEY=...   # the demo key from https://t.me/apiautospace_bot?start=demo
//	go run ./examples/search
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
	page, err := client.Cars(context.Background(), apiauto.MarketKR, &apiauto.CarsParams{
		Brand:   "Kia",
		Model:   "Sorento",
		YearMin: 2021,
		Limit:   5,
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, car := range page.Items {
		fmt.Println(car.ID, car.Title, car.Price.Amount, car.Price.Currency)
	}
	if left := client.LastResponse().QuotaRemaining; left != nil {
		fmt.Println("records left:", *left)
	}
}
