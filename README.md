# apiauto-go — Go client for apiauto.space

The Go client of the [apiauto.space](https://apiauto.space/en/) REST API: used-car listings from **Encar**
(South Korea), **Che168** (China) and the **Copart** and **IAAI** auctions (USA) in one JSON record format.
Go 1.23+, only the standard library.

Encar and Che168 publish no public API for their listings that we could find; Copart and IAAI have private
partner integrations, not a public self-serve API for lots. apiauto.space is an independent service.

The API is paid from $59 a month for 100,000 cars. A free demo key works for 1 day: 100 requests, up to 20 cars
per request, no card, from the Telegram bot [@apiautospace_bot](https://t.me/apiautospace_bot?start=demo).

```go
client, err := apiauto.NewClient() // reads APIAUTO_KEY
if err != nil {
	log.Fatal(err)
}
page, err := client.Cars(ctx, apiauto.MarketKR, &apiauto.CarsParams{Brand: "Kia", Model: "Sorento", YearMin: 2021, Limit: 5})
if err != nil {
	log.Fatal(err)
}
for _, car := range page.Items {
	fmt.Println(car.ID, car.Title, car.Price.Amount, car.Price.Currency)
}
```

A real output line:

```text
encar:43078256 2023 Kia Sorento 4 Gen HEV 1.6 2WD Noblesse 28500000.00 KRW
```

Last verified: 2026-11-02, unit and live tests run on Go 1.23 and 1.27.

## Quick start

```bash
go get github.com/apiauto-space/apiauto-go
export APIAUTO_KEY=your-key
```

Keep the key on your server: the API does not accept browser requests. Documentation:
[apiauto.space/en/docs](https://apiauto.space/en/docs/), [OpenAPI](https://api.apiauto.space/openapi.json).

```go
// Every car, following the cursor; the last argument stops after that many, 0 means no limit.
params := &apiauto.CarsParams{Brand: "BYD", Limit: 100}
for car, err := range client.IterCars(ctx, apiauto.MarketCN, params, 1000) {
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(car.ID)
}

car, err := client.Car(ctx, "encar:43020893")

since := time.Now().Add(-time.Hour)
for event, err := range client.IterChanges(ctx, apiauto.MarketKR, since, &apiauto.ChangesParams{Limit: 100}, 0) {
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(event.Type, event.CarID, event.NewPrice)
}

client.Brands(ctx, apiauto.MarketCN) // free: also Models, Dictionaries, Markets, Usage
client.LastResponse()                // QuotaRemaining, RateRemaining, DataAgeSeconds...
```

The limit asks only for what is still needed, so the last page spends no extra quota. Iteration ends with
`ErrCursorLoop` if the API repeats a cursor. `USLive` searches Copart and IAAI live at the data provider (Pro and
Max, a separate monthly counter). Runnable programs are in
[examples/search](https://github.com/apiauto-space/apiauto-go/blob/main/examples/search/main.go),
[us-lots](https://github.com/apiauto-space/apiauto-go/blob/main/examples/us-lots/main.go) and
[change-feed](https://github.com/apiauto-space/apiauto-go/blob/main/examples/change-feed/main.go).

## Fields

`apiauto.Car` has `ID`, `Market`, `Source`, `SourceID`, `Title`, `Brand`, `Model`, `Generation`, `Trim`, `Year`,
`MileageKM`, `Fuel`, `Transmission`, `EngineCC`, `BodyType`, `Color`, `VIN`, `Price`, `PriceUSD`, `Location`,
`Photos`, `PhotoCount`, `ListingURL`, `IsActive`, `FirstSeenAt`, `LastSeenAt`, `DisappearedAt`, `UpdatedAt`,
`SourceMode`, `Raw`. A text field the source does not give is empty, a number is `nil`. `CarsParams` has the filters
of the API: `Brand`, `Model`, `Generation`, `Trim`, `YearMin`, `YearMax`, `PriceMin`, `PriceMax`, `PriceUSDMin`,
`PriceUSDMax`, `MileageMax`, `Fuel`, `Transmission`, `BodyType`, `HasPhotos`, `IsActive`, `Q`, `Sort`, `Limit`, `Cursor`.

## Units and gotchas

- Prices are decimal strings in the market's currency, so no precision is lost. `PriceUSD` uses the ECB daily rate.
- Mileage is kilometres. Photo links are signed and live about a day: store the `ID`.
- **Copart or IAAI only:** `MarketUS` holds both and the API has no filter by auction, so keep records by `car.Source`.
  The quota counts every lot returned. Lots come in loading order: on 2 November 2026 `SortOldest` put Copart first and
  the default sort put IAAI first. That is no contract.
- A `ChangeDisappeared` event does not mean sold. Listing data is untrusted: escape it in HTML.

## Data freshness

The refresh target is hourly. That is a target, not a promise: the real age is in `client.Markets` and in
`LastResponse().DataAgeSeconds`.

## Limits

Up to 100 cars per request (20 on the demo). Paid plans count cars returned; the demo counts data requests.

| HTTP | Sentinel for `errors.Is` | Retried |
| --- | --- | --- |
| 400, 401, 402, 403, 404 | `ErrBadRequest`, `ErrAuthentication`, `ErrQuotaExceeded`, `ErrPermissionDenied`, `ErrNotFound` | no |
| 429, 503 | `ErrRateLimit`, `ErrServiceUnavailable` | yes, `Retry-After` up to 60 s |
| other 5xx, network | `ErrServer`, `ErrConnection` | yes, backoff |

`errors.As` gives an `*apiauto.Error` with `StatusCode`, `Code`, `Detail`, `RetryAfter` and `ResetAt`. Two retries by
default. Security behaviour and its limits are in
[SECURITY.md](https://github.com/apiauto-space/apiauto-go/blob/main/SECURITY.md).

## Pricing

Demo free for 1 day (100 requests); Start $59 a month (100,000 cars, 1 market); Pro $149 (500,000 cars, 2 markets,
change feed); Max $299 (2,000,000 cars, 3 markets). Current prices: [apiauto.space/en/#pricing](https://apiauto.space/en/#pricing).

## Other languages

| Language | Package | Repository |
| --- | --- | --- |
| Python | `pip install apiauto-space` | [apiauto-python](https://github.com/apiauto-space/apiauto-python) |
| JavaScript, TypeScript | `npm install apiauto` | [apiauto-js](https://github.com/apiauto-space/apiauto-js) |
| PHP | `composer require apiauto/apiauto` | [apiauto-php](https://github.com/apiauto-space/apiauto-php) |
| curl and examples | — | [Encar](https://github.com/apiauto-space/encar-api-examples), [Che168](https://github.com/apiauto-space/che168-api-examples), [Copart](https://github.com/apiauto-space/copart-api-examples), [IAAI](https://github.com/apiauto-space/iaai-api-examples) |

## Not affiliated

This client and apiauto.space are not affiliated with, endorsed by or sponsored by Encar, Che168, Autohome, Copart
or IAAI; the names only say where the listings come from. Use of the data is subject to the
[apiauto.space terms](https://apiauto.space/en/offer/). Code: [MIT](https://github.com/apiauto-space/apiauto-go/blob/main/LICENSE).
