// What changed on Che168 in the last 24 hours: new cars, removed cars, price changes.
//
// The feed comes with the Pro, Max and demo plans; each event counts as one record.
//
//	export APIAUTO_KEY=...
//	go run ./examples/change-feed
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	apiauto "github.com/apiauto-space/apiauto-go"
)

func main() {
	client, err := apiauto.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	since := time.Now().Add(-24 * time.Hour)
	params := &apiauto.ChangesParams{Limit: 5}
	for event, err := range client.IterChanges(context.Background(), apiauto.MarketCN, since, params, 5) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(event.At.Format(time.RFC3339), event.Type, event.CarID, event.NewPrice)
	}
}
