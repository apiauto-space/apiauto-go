// Package apiauto is a client for the apiauto.space REST API: used-car listings from Encar
// (South Korea), Che168 (China) and the Copart and IAAI auctions (USA) in one JSON record format.
//
// It uses only the standard library. The key comes from the APIAUTO_KEY environment variable or
// from [WithAPIKey]; a free demo key comes from https://t.me/apiautospace_bot?start=demo.
//
//	client, err := apiauto.NewClient()
//	if err != nil {
//		log.Fatal(err)
//	}
//	page, err := client.Cars(ctx, apiauto.MarketKR, &apiauto.CarsParams{Brand: "Kia", Limit: 20})
//
// Failed requests return an [*Error]; test it with [errors.Is] against [ErrNotFound],
// [ErrQuotaExceeded] and the other sentinels, or read its fields with [errors.As].
//
// The client is safe for concurrent use. It is unofficial: the project is not affiliated with
// Encar, Che168/Autohome, Copart or IAAI.
package apiauto
