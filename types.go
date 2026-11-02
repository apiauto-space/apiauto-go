package apiauto

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Market is a market code.
type Market string

const (
	// MarketKR is Encar, South Korea.
	MarketKR Market = "kr"
	// MarketCN is Che168, China.
	MarketCN Market = "cn"
	// MarketUS is the Copart and IAAI auctions, USA. The API has no filter by auction: keep the
	// records you need by [Car.Source], and the events of the feed by the prefix of [Change.CarID].
	MarketUS Market = "us"
)

func (m Market) check() error {
	switch m {
	case MarketKR, MarketCN, MarketUS:
		return nil
	}
	return fmt.Errorf("apiauto: market must be one of kr, cn, us, got %q", string(m))
}

// Sort is the order of [Client.Cars]; SortNewest by default.
type Sort string

// The sorts the API accepts.
const (
	SortNewest      Sort = "-first_seen"
	SortOldest      Sort = "first_seen"
	SortPrice       Sort = "price"
	SortPriceDesc   Sort = "-price"
	SortYear        Sort = "year"
	SortYearDesc    Sort = "-year"
	SortMileage     Sort = "mileage"
	SortMileageDesc Sort = "-mileage"
)

// ChangeType is an event type of the change feed.
type ChangeType string

// The event types of the change feed.
const (
	ChangeAdded       ChangeType = "added"
	ChangeDisappeared ChangeType = "disappeared"
	ChangePriceDrop   ChangeType = "price_drop"
	ChangePriceUp     ChangeType = "price_up"
)

// Dictionaries is field name, then canonical value, then synonyms.
type Dictionaries map[string]map[string][]string

// Price is a price in the market's currency.
type Price struct {
	// Amount is a decimal string such as "23900000.00", kept as sent so no precision is lost;
	// empty when the source gives no price.
	Amount string `json:"amount"`
	// Currency is KRW, CNY or USD.
	Currency string `json:"currency"`
}

// Location is where the car is.
type Location struct {
	// Country is an ISO 3166 code: KR, CN or US.
	Country string `json:"country"`
	// Region is a province or a US state.
	Region string `json:"region"`
	City   string `json:"city"`
}

// Car is one car record, the same fields in every market. A text field the source does not give
// is empty and a number is nil, never a guess. Photos are signed links that work for about a day:
// store the ID, not the links.
type Car struct {
	// ID is source:source_id, such as encar:43020893, and never changes.
	ID     string `json:"id"`
	Market Market `json:"market"`
	// Source is encar, che168, copart or iaai.
	Source string `json:"source"`
	// SourceID is the listing or lot number at the source.
	SourceID string `json:"source_id"`
	Title    string `json:"title"`
	Brand    string `json:"brand"`
	Model    string `json:"model"`
	// Generation is set in Korea only.
	Generation   string `json:"generation"`
	Trim         string `json:"trim"`
	Year         *int   `json:"year"`
	MileageKM    *int   `json:"mileage_km"`
	Fuel         string `json:"fuel"`
	Transmission string `json:"transmission"`
	EngineCC     *int   `json:"engine_cc"`
	BodyType     string `json:"body_type"`
	Color        string `json:"color"`
	VIN          string `json:"vin"`
	Price        Price  `json:"price"`
	// PriceUSD is the price in dollars at the ECB daily rate, a decimal string.
	PriceUSD      string     `json:"price_usd"`
	Location      Location   `json:"location"`
	Photos        []string   `json:"photos"`
	PhotoCount    int        `json:"photo_count"`
	ListingURL    string     `json:"listing_url"`
	IsActive      bool       `json:"is_active"`
	FirstSeenAt   time.Time  `json:"first_seen_at"`
	LastSeenAt    time.Time  `json:"last_seen_at"`
	DisappearedAt *time.Time `json:"disappeared_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	// SourceMode is replica, or live for [Client.USLive].
	SourceMode string `json:"source_mode"`
	// Raw holds the fields only the source has, as it sends them, such as damage_main or sale_date.
	Raw map[string]any `json:"raw"`
}

// RawString returns a text field of Raw, empty when it is missing or not text.
func (c Car) RawString(key string) string {
	value, _ := c.Raw[key].(string)
	return value
}

// CarsPage is a page of [Client.Cars].
type CarsPage struct {
	Items []Car `json:"items"`
	// NextCursor goes into [CarsParams.Cursor] for the next page; empty on the last one.
	NextCursor string `json:"next_cursor"`
	// Count is the number of cars matching the filters when there are at most 10,000; nil above.
	Count *int `json:"count"`
}

// Change is one event of the change feed.
type Change struct {
	ID int64 `json:"id"`
	// CarID is the car's id; its prefix tells the source.
	CarID  string     `json:"car_id"`
	Market Market     `json:"market"`
	Type   ChangeType `json:"type"`
	At     time.Time  `json:"at"`
	// OldPrice and NewPrice are decimal strings, set on price events only.
	OldPrice string `json:"old_price"`
	NewPrice string `json:"new_price"`
	// Pct is the size of the price change in percent, always positive; the direction is in Type.
	Pct string `json:"pct"`
}

// ChangesPage is a page of [Client.Changes].
type ChangesPage struct {
	Items []Change `json:"items"`
	// NextCursor goes into [ChangesParams.Cursor] for the next page; empty on the last one.
	NextCursor string `json:"next_cursor"`
}

// ReferenceItem is a make or model as the catalog spells it, with the number of active cars.
type ReferenceItem struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// MarketStatus is the plan access and data freshness of one market.
type MarketStatus struct {
	Market            Market     `json:"market"`
	Source            string     `json:"source"`
	AvailableInPlan   bool       `json:"available_in_plan"`
	Status            string     `json:"status"`
	DataUpdatedAt     *time.Time `json:"data_updated_at"`
	DataAgeSeconds    *int       `json:"data_age_seconds"`
	StaleAfterSeconds int        `json:"stale_after_seconds"`
	Stale             bool       `json:"stale"`
	ActiveCars        int        `json:"active_cars"`
}

// UsageMarket is the usage of one market in the current period.
type UsageMarket struct {
	Market         Market `json:"market"`
	Records        int    `json:"records"`
	Requests       int    `json:"requests"`
	LiveUSRequests int    `json:"live_us_requests"`
}

// Usage is the plan, its term and how much of the quota is spent.
type Usage struct {
	Plan string `json:"plan"`
	// Status is demo, active or grace.
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
	// Period is YYYY-MM, or demo for the whole demo term.
	Period  string    `json:"period"`
	ResetAt time.Time `json:"reset_at"`
	// QuotaUnit is records on paid plans, requests on the demo.
	QuotaUnit    string        `json:"quota_unit"`
	QuotaLimit   int           `json:"quota_limit"`
	QuotaUsed    int           `json:"quota_used"`
	RecordsLimit int           `json:"records_limit"`
	RecordsUsed  int           `json:"records_used"`
	LiveUSLimit  int           `json:"live_us_limit"`
	LiveUSUsed   int           `json:"live_us_used"`
	ByMarket     []UsageMarket `json:"by_market"`
}

// ResponseInfo is the quota and rate-limit headers of a response; a header the response did not
// carry is nil or empty.
type ResponseInfo struct {
	Status int
	// QuotaLimit is the period quota (X-Quota-Limit).
	QuotaLimit *int
	// QuotaRemaining is what is left of it (X-Quota-Remaining).
	QuotaRemaining *int
	// QuotaReset is when it renews, ISO 8601 (X-Quota-Reset).
	QuotaReset string
	// RateLimit is the requests per second the plan allows (X-RateLimit-Limit).
	RateLimit *int
	// RateRemaining is the requests left in the current second (X-RateLimit-Remaining).
	RateRemaining *int
	// RateReset is the seconds until the rate window resets (X-RateLimit-Reset).
	RateReset *int
	// DataAgeSeconds is the age of the market copy that answered (X-Data-Age-Seconds).
	DataAgeSeconds *int
}

func responseInfoFrom(response *http.Response) *ResponseInfo {
	number := func(name string) *int {
		value, err := strconv.Atoi(response.Header.Get(name))
		if err != nil {
			return nil
		}
		return &value
	}
	return &ResponseInfo{
		Status:         response.StatusCode,
		QuotaLimit:     number("X-Quota-Limit"),
		QuotaRemaining: number("X-Quota-Remaining"),
		QuotaReset:     response.Header.Get("X-Quota-Reset"),
		RateLimit:      number("X-RateLimit-Limit"),
		RateRemaining:  number("X-RateLimit-Remaining"),
		RateReset:      number("X-RateLimit-Reset"),
		DataAgeSeconds: number("X-Data-Age-Seconds"),
	}
}

// CarsParams are the optional filters of [Client.Cars]. A zero value is left out of the request;
// use [Bool] for the two boolean filters.
type CarsParams struct {
	// Brand is a make as [Client.Brands] spells it; case, spaces and hyphens are ignored.
	Brand string
	// Model is the model family, one name in every market: Grandeur, 3 Series.
	Model string
	// Generation is for Korea only: the generation as in [Car.Generation].
	Generation string
	// Trim is an exact match.
	Trim    string
	YearMin int
	YearMax int
	// PriceMin and PriceMax are in the market's currency (KRW, CNY or USD).
	PriceMin float64
	PriceMax float64
	// PriceUSDMin and PriceUSDMax are in dollars at the ECB rate.
	PriceUSDMin float64
	PriceUSDMax float64
	// MileageMax is in kilometres.
	MileageMax int
	// Fuel, Transmission and BodyType are canonical values from [Client.Dictionaries].
	Fuel         string
	Transmission string
	BodyType     string
	HasPhotos    *bool
	// IsActive is true by default on the server; false returns cars taken off sale in the last 14 days.
	IsActive *bool
	// Q is up to 6 words of the title: year, make, model, trim.
	Q    string
	Sort Sort
	// Limit is the cars per page, 1-100 (up to 20 on the demo); 20 by default.
	Limit int
	// Cursor is [CarsPage.NextCursor] of the previous page.
	Cursor string
}

// ChangesParams are the optional parameters of [Client.Changes].
type ChangesParams struct {
	// Types are the event types to return; every type when empty.
	Types []ChangeType
	// MinPct is the smallest price change in percent (0.5 by default on the server).
	MinPct float64
	// Limit is the events per page, 1-100.
	Limit int
	// Cursor is [ChangesPage.NextCursor] of the previous page.
	Cursor string
}

// USLiveParams are the optional parameters of [Client.USLive].
type USLiveParams struct {
	Make    string
	Model   string
	YearMin int
	YearMax int
	// PriceMax is in US dollars.
	PriceMax int
	// Page is 1-100.
	Page int
}

// Bool returns a pointer to value, for [CarsParams.HasPhotos] and [CarsParams.IsActive].
func Bool(value bool) *bool { return &value }

func setString(query url.Values, name, value string) {
	if value != "" {
		query.Set(name, value)
	}
}

func setInt(query url.Values, name string, value int) {
	if value != 0 {
		query.Set(name, strconv.Itoa(value))
	}
}

func setFloat(query url.Values, name string, value float64) {
	if value != 0 {
		query.Set(name, strconv.FormatFloat(value, 'f', -1, 64))
	}
}

func setBool(query url.Values, name string, value *bool) {
	if value != nil {
		query.Set(name, strconv.FormatBool(*value))
	}
}

func carsQuery(market Market, p *CarsParams) (url.Values, error) {
	if err := market.check(); err != nil {
		return nil, err
	}
	query := url.Values{"market": {string(market)}}
	if p == nil {
		return query, nil
	}
	setString(query, "brand", p.Brand)
	setString(query, "model", p.Model)
	setString(query, "generation", p.Generation)
	setString(query, "trim", p.Trim)
	setInt(query, "year_min", p.YearMin)
	setInt(query, "year_max", p.YearMax)
	setFloat(query, "price_min", p.PriceMin)
	setFloat(query, "price_max", p.PriceMax)
	setFloat(query, "price_usd_min", p.PriceUSDMin)
	setFloat(query, "price_usd_max", p.PriceUSDMax)
	setInt(query, "mileage_max", p.MileageMax)
	setString(query, "fuel", p.Fuel)
	setString(query, "transmission", p.Transmission)
	setString(query, "body_type", p.BodyType)
	setBool(query, "has_photos", p.HasPhotos)
	setBool(query, "is_active", p.IsActive)
	setString(query, "q", p.Q)
	setString(query, "sort", string(p.Sort))
	setInt(query, "limit", p.Limit)
	setString(query, "cursor", p.Cursor)
	return query, nil
}

func changesQuery(market Market, since time.Time, p *ChangesParams) (url.Values, error) {
	if err := market.check(); err != nil {
		return nil, err
	}
	if since.IsZero() {
		return nil, fmt.Errorf("apiauto: since must be set")
	}
	query := url.Values{
		"market": {string(market)},
		"since":  {since.UTC().Format("2006-01-02T15:04:05.999999Z07:00")},
	}
	if p == nil {
		return query, nil
	}
	for _, changeType := range p.Types {
		query.Add("type", string(changeType))
	}
	setFloat(query, "min_pct", p.MinPct)
	setInt(query, "limit", p.Limit)
	setString(query, "cursor", p.Cursor)
	return query, nil
}

func (p *USLiveParams) query() url.Values {
	query := url.Values{}
	if p == nil {
		return query
	}
	setString(query, "make", p.Make)
	setString(query, "model", p.Model)
	setInt(query, "year_min", p.YearMin)
	setInt(query, "year_max", p.YearMax)
	setInt(query, "price_max", p.PriceMax)
	setInt(query, "page", p.Page)
	return query
}
