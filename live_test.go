package apiauto

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// liveClient returns a client for the real API, or skips the test unless APIAUTO_KEY is set.
//
// The live tests spend at most 10 records of the quota (5 data requests on a demo key): references
// and usage are free, the rest asks for 1-2 cars or events at a time. They never call USLive, which
// spends the paid counter of the data provider.
//
//	APIAUTO_KEY=... go test -run Live -count=1 ./...
func liveClient(t *testing.T) *Client {
	t.Helper()
	if strings.TrimSpace(os.Getenv(APIKeyEnv)) == "" {
		t.Skip("set APIAUTO_KEY to run the live smoke test")
	}
	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestLiveFreeEndpoints(t *testing.T) {
	client := liveClient(t)
	ctx := context.Background()
	usage, err := client.Usage(ctx)
	if err != nil || (usage.QuotaUnit != "records" && usage.QuotaUnit != "requests") {
		t.Fatalf("usage = %+v, %v", usage, err)
	}
	markets, err := client.Markets(ctx)
	if err != nil || len(markets) != 3 {
		t.Fatalf("markets = %+v, %v", markets, err)
	}
	brands, err := client.Brands(ctx, MarketCN)
	if err != nil || len(brands) == 0 {
		t.Fatalf("brands = %+v, %v", brands, err)
	}
	models, err := client.Models(ctx, MarketCN, brands[0].Name)
	if err != nil || len(models) == 0 {
		t.Fatalf("models = %+v, %v", models, err)
	}
	dictionaries, err := client.Dictionaries(ctx)
	if err != nil || dictionaries["fuel"] == nil {
		t.Fatalf("dictionaries = %+v, %v", dictionaries, err)
	}
}

func TestLiveCarsAndCarByID(t *testing.T) {
	client := liveClient(t)
	ctx := context.Background()
	page, err := client.Cars(ctx, MarketCN, &CarsParams{Limit: 2})
	if err != nil || len(page.Items) == 0 || len(page.Items) > 2 {
		t.Fatalf("page = %+v, %v", page, err)
	}
	first := page.Items[0]
	if !strings.HasPrefix(first.ID, "che168:") {
		t.Errorf("id = %s", first.ID)
	}
	info := client.LastResponse()
	if info.Status != 200 || info.DataAgeSeconds == nil {
		t.Errorf("info = %+v", info)
	}
	car, err := client.Car(ctx, first.ID)
	if err != nil || car.ID != first.ID {
		t.Fatalf("car = %+v, %v", car, err)
	}
}

func TestLiveIteration(t *testing.T) {
	client := liveClient(t)
	seen := 0
	for car, err := range client.IterCars(context.Background(), MarketUS, &CarsParams{Limit: 2}, 3) {
		if err != nil {
			t.Fatal(err)
		}
		if car.Source != "copart" && car.Source != "iaai" {
			t.Errorf("source = %s", car.Source)
		}
		seen++
	}
	if seen != 3 {
		t.Errorf("seen = %d", seen)
	}
}

func TestLiveChangeFeed(t *testing.T) {
	client := liveClient(t)
	page, err := client.Changes(context.Background(), MarketCN, time.Now().Add(-24*time.Hour), &ChangesParams{Limit: 1})
	if err != nil || len(page.Items) > 1 {
		t.Fatalf("page = %+v, %v", page, err)
	}
}

func TestLiveNotFound(t *testing.T) {
	client := liveClient(t)
	if _, err := client.Car(context.Background(), "encar:1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v", err)
	}
}
