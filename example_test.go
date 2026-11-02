package apiauto_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	apiauto "github.com/apiauto-space/apiauto-go"
)

func ExampleClient_Cars() {
	client, err := apiauto.NewClient() // reads APIAUTO_KEY
	if err != nil {
		log.Fatal(err)
	}
	page, err := client.Cars(context.Background(), apiauto.MarketKR, &apiauto.CarsParams{
		Brand:   "Kia",
		Model:   "Sorento",
		YearMin: 2021,
		Limit:   20,
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, car := range page.Items {
		fmt.Println(car.ID, car.Title, car.Price.Amount, car.Price.Currency)
	}
}

// market=us holds both auctions and the API has no filter by auction: keep the lots you need by
// Source. The quota counts every lot returned.
func ExampleClient_IterCars() {
	client, err := apiauto.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	params := &apiauto.CarsParams{Brand: "Toyota", Limit: 100}
	for lot, err := range client.IterCars(context.Background(), apiauto.MarketUS, params, 300) {
		if err != nil {
			log.Fatal(err)
		}
		if lot.Source == "copart" {
			fmt.Println(lot.ID, lot.VIN, lot.RawString("damage_main"), lot.RawString("sale_date"))
		}
	}
}

func ExampleClient_IterChanges() {
	client, err := apiauto.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	since := time.Now().Add(-time.Hour)
	for event, err := range client.IterChanges(context.Background(), apiauto.MarketCN, since, nil, 0) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(event.Type, event.CarID, event.NewPrice)
	}
}

func ExampleError() {
	client, err := apiauto.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	_, err = client.Car(context.Background(), "encar:1")
	var apiErr *apiauto.Error
	switch {
	case errors.Is(err, apiauto.ErrNotFound):
		fmt.Println("no such car")
	case errors.Is(err, apiauto.ErrQuotaExceeded) && errors.As(err, &apiErr):
		fmt.Println("quota renews at", apiErr.ResetAt)
	case errors.As(err, &apiErr):
		fmt.Println(apiErr.StatusCode, apiErr.Code, apiErr.Detail)
	}
}
