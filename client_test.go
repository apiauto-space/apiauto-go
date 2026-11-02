package apiauto

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode"
)

const testKey = "test-key-1234567890"

// answer is one canned response of the fake API.
type answer struct {
	status  int
	headers map[string]string
	body    string
}

// fakeAPI serves answers in order and records the requests.
type fakeAPI struct {
	t        *testing.T
	answers  []answer
	requests []*http.Request
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.requests = append(f.requests, r)
	if len(f.requests) > len(f.answers) {
		f.t.Errorf("unexpected request: %s", r.URL)
		w.WriteHeader(http.StatusTeapot)
		return
	}
	reply := f.answers[len(f.requests)-1]
	for name, value := range reply.headers {
		w.Header().Set(name, value)
	}
	w.WriteHeader(reply.status)
	fmt.Fprint(w, reply.body)
}

// newTestClient starts a fake API and a client whose retries record the wait instead of sleeping.
func newTestClient(t *testing.T, answers ...answer) (*Client, *fakeAPI, *[]time.Duration) {
	t.Helper()
	api := &fakeAPI{t: t, answers: answers}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	client, err := NewClient(WithAPIKey(testKey), WithBaseURL(server.URL+"/v1"))
	if err != nil {
		t.Fatal(err)
	}
	waits := &[]time.Duration{}
	client.sleep = func(_ context.Context, delay time.Duration) error {
		*waits = append(*waits, delay)
		return nil
	}
	return client, api, waits
}

func ok(body string) answer { return answer{status: 200, body: body} }

func fixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestReadsTheKeyFromTheEnvironment(t *testing.T) {
	t.Setenv(APIKeyEnv, " env-key ")
	api := &fakeAPI{t: t, answers: []answer{ok(`{"plan":"max"}`)}}
	server := httptest.NewServer(api)
	defer server.Close()
	client, err := NewClient(WithBaseURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Usage(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := api.requests[0].Header.Get("X-API-Key"); got != "env-key" {
		t.Errorf("key = %q", got)
	}
}

func TestFailsWithoutAKey(t *testing.T) {
	t.Setenv(APIKeyEnv, "")
	_, err := NewClient()
	if err == nil || !strings.Contains(err.Error(), demoKeyURL) {
		t.Errorf("err = %v", err)
	}
}

func TestRefusesPlainHTTPExceptLocalhost(t *testing.T) {
	for _, good := range []string{"https://api.apiauto.space/v1", "http://127.0.0.1:8080", "http://localhost/v1", "http://[::1]:80"} {
		if _, err := NewClient(WithAPIKey(testKey), WithBaseURL(good)); err != nil {
			t.Errorf("%s: %v", good, err)
		}
	}
	for _, bad := range []string{
		"http://api.apiauto.space/v1",
		"ftp://localhost",
		"api.apiauto.space",
		"https://user:secret@api.apiauto.space/v1",
		"https://api.apiauto.space/v1?a=1",
		"https://api.apiauto.space/v1?",
		"https://api.apiauto.space/v1#a",
		"https://user:secret@bad host/",
	} {
		_, err := NewClient(WithAPIKey(testKey), WithBaseURL(bad))
		if err == nil {
			t.Errorf("%s: no error", bad)
		} else if strings.Contains(err.Error(), "secret") {
			t.Errorf("%s: the error shows the URL: %v", bad, err)
		}
	}
}

func TestOptionsAreChecked(t *testing.T) {
	for name, option := range map[string]Option{
		"timeout": WithTimeout(0),
		"retries": WithMaxRetries(-1),
		"http":    WithHTTPClient(nil),
	} {
		if _, err := NewClient(WithAPIKey(testKey), option); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestPrintingNeverShowsTheKey(t *testing.T) {
	client, err := NewClient(WithAPIKey(testKey))
	if err != nil {
		t.Fatal(err)
	}
	value := *client
	type byPointer struct{ C *Client }
	type byValue struct{ C Client }
	args := map[string]any{
		"pointer":          client,
		"value":            value,
		"pointer to value": &value,
		"holder pointer":   byPointer{client},
		"holder value":     byValue{value},
		"holder ref":       &byValue{value},
		"slice":            []*Client{client},
		"map":              map[string]*Client{"a": client},
	}
	hexKey := fmt.Sprintf("%x", testKey)
	upperKey := fmt.Sprintf("%X", testKey)
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%t", "%p", "%e", "%E", "%f", "%F", "%g", "%G", "%U", "%c", "%b", "%o", "%O", "%T"} {
		for name, arg := range args {
			got := fmt.Sprintf(verb, arg)
			if strings.Contains(got, testKey) || strings.Contains(got, hexKey) || strings.Contains(got, upperKey) {
				t.Errorf("%s of %s shows the key: %.120s", verb, name, got)
			}
		}
	}
	if data, _ := json.Marshal(client); strings.Contains(string(data), testKey) {
		t.Errorf("json shows the key: %s", data)
	}
}

func TestACopyOfTheClientStillWorks(t *testing.T) {
	client, api, _ := newTestClient(t, ok(`{"plan":"max"}`))
	value := *client
	if _, err := value.Usage(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := api.requests[0].Header.Get("X-API-Key"); got != testKey {
		t.Errorf("key = %q", got)
	}
}

func TestRejectsAKeyAHeaderCannotCarry(t *testing.T) {
	for _, key := range []string{"abc def", "abc\r\nX-Evil: 1", "ключ", "abc\x7f"} {
		_, err := NewClient(WithAPIKey(key))
		if err == nil {
			t.Errorf("%q accepted", key)
		} else if strings.Contains(err.Error(), key) {
			t.Errorf("the error shows the key: %v", err)
		}
	}
}

func TestErrorsDoNotCarryTheKey(t *testing.T) {
	client, _, _ := newTestClient(t, answer{status: 401, body: `{"error":"unauthorized"}`})
	_, err := client.Usage(context.Background())
	if err == nil {
		t.Fatal("no error")
	}
	if text := fmt.Sprintf("%v %+v %#v", err, err, err); strings.Contains(text, testKey) {
		t.Errorf("the error shows the key: %s", text)
	}
}

func TestSendsHeadersAndBuildsTheQuery(t *testing.T) {
	client, api, _ := newTestClient(t, ok(`{"items":[],"next_cursor":null,"count":0}`))
	page, err := client.Cars(context.Background(), MarketKR, &CarsParams{
		Brand:       "Mercedes-Benz",
		Model:       "3 Series",
		YearMin:     2020,
		PriceUSDMax: 19999.5,
		HasPhotos:   Bool(true),
		IsActive:    Bool(false),
		Sort:        SortPrice,
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Count == nil || *page.Count != 0 || page.NextCursor != "" {
		t.Errorf("page = %+v", page)
	}
	request := api.requests[0]
	if request.URL.Path != "/v1/cars" {
		t.Errorf("path = %s", request.URL.Path)
	}
	want := map[string]string{
		"market":        "kr",
		"brand":         "Mercedes-Benz",
		"model":         "3 Series",
		"year_min":      "2020",
		"price_usd_max": "19999.5",
		"has_photos":    "true",
		"is_active":     "false",
		"sort":          "price",
	}
	query := request.URL.Query()
	if len(query) != len(want) {
		t.Errorf("query = %v", query)
	}
	for name, value := range want {
		if query.Get(name) != value {
			t.Errorf("%s = %q, want %q", name, query.Get(name), value)
		}
	}
	if got := request.Header.Get("X-API-Key"); got != testKey {
		t.Errorf("key = %q", got)
	}
	if got := request.Header.Get("Accept"); got != "application/json" {
		t.Errorf("accept = %q", got)
	}
	if got := request.Header.Get("User-Agent"); !strings.HasPrefix(got, "apiauto-go/"+Version) {
		t.Errorf("user agent = %q", got)
	}
	if strings.Contains(request.URL.String(), testKey) {
		t.Error("the key is in the URL")
	}
}

func TestRejectsABadMarketWithoutARequest(t *testing.T) {
	client, api, _ := newTestClient(t)
	if _, err := client.Cars(context.Background(), Market("de"), nil); err == nil {
		t.Error("no error")
	}
	for _, err := range client.IterCars(context.Background(), Market("de"), nil, 0) {
		if err == nil {
			t.Error("no error")
		}
	}
	if len(api.requests) != 0 {
		t.Errorf("%d requests", len(api.requests))
	}
}

func TestCarIDIsEscapedExceptTheColon(t *testing.T) {
	client, api, _ := newTestClient(t, ok(fixture(t, "che168-car.json")), ok(`{"id":"x"}`))
	if _, err := client.Car(context.Background(), "che168:60385167"); err != nil {
		t.Fatal(err)
	}
	if got := api.requests[0].RequestURI; got != "/v1/cars/che168:60385167" {
		t.Errorf("uri = %s", got)
	}
	if _, err := client.Car(context.Background(), "a/b?c"); err != nil {
		t.Fatal(err)
	}
	if got := api.requests[1].RequestURI; got != "/v1/cars/a%2Fb%3Fc" {
		t.Errorf("uri = %s", got)
	}
	for _, bad := range []string{" ", ".", ".."} {
		if _, err := client.Car(context.Background(), bad); err == nil {
			t.Errorf("no error for the id %q", bad)
		}
	}
	if len(api.requests) != 2 {
		t.Errorf("%d requests", len(api.requests))
	}
}

func TestDecodesARealCheCar(t *testing.T) {
	client, _, _ := newTestClient(t, ok(fixture(t, "che168-car.json")))
	car, err := client.Car(context.Background(), "che168:60385167")
	if err != nil {
		t.Fatal(err)
	}
	if car.Market != MarketCN || car.Source != "che168" || car.SourceID != "60385167" || car.Brand != "BYD" {
		t.Errorf("car = %+v", car)
	}
	if car.VIN != "" || car.Price.Amount != "79000.00" || car.Price.Currency != "CNY" || car.Location.Country != "CN" {
		t.Errorf("car = %+v", car)
	}
	if car.Year == nil || *car.Year != 2025 || !car.IsActive || car.SourceMode != "replica" || car.DisappearedAt != nil {
		t.Errorf("car = %+v", car)
	}
	if car.FirstSeenAt.IsZero() || car.RawString("drive") != "fwd" || car.PhotoCount != 24 {
		t.Errorf("car = %+v", car)
	}
}

func TestDecodesARealAuctionLot(t *testing.T) {
	client, _, _ := newTestClient(t, ok(fixture(t, "copart-lot.json")))
	lot, err := client.Car(context.Background(), "copart:70370576")
	if err != nil {
		t.Fatal(err)
	}
	if lot.Market != MarketUS || lot.Source != "copart" || lot.VIN == "" || lot.ListingURL != "" {
		t.Errorf("lot = %+v", lot)
	}
	if lot.RawString("sale_date") == "" {
		t.Errorf("raw = %v", lot.Raw)
	}
}

func TestIteratesPagesAndKeepsTheLastPageSmall(t *testing.T) {
	client, api, _ := newTestClient(t,
		ok(`{"items":[{"id":"a:1"},{"id":"a:2"}],"next_cursor":"next+1","count":9}`),
		ok(`{"items":[{"id":"a:3"}],"next_cursor":"next2","count":9}`),
	)
	var ids []string
	for car, err := range client.IterCars(context.Background(), MarketKR, &CarsParams{Limit: 2}, 3) {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, car.ID)
	}
	if strings.Join(ids, ",") != "a:1,a:2,a:3" {
		t.Errorf("ids = %v", ids)
	}
	if len(api.requests) != 2 {
		t.Fatalf("%d requests", len(api.requests))
	}
	if got := api.requests[0].URL.Query().Get("limit"); got != "2" {
		t.Errorf("first limit = %s", got)
	}
	second := api.requests[1].URL.Query()
	if second.Get("limit") != "1" || second.Get("cursor") != "next+1" {
		t.Errorf("second query = %v", second)
	}
}

func TestIterationEndsOnTheLastOrAnEmptyPage(t *testing.T) {
	count := func(client *Client) int {
		seen := 0
		for _, err := range client.IterCars(context.Background(), MarketUS, nil, 0) {
			if err != nil {
				t.Fatal(err)
			}
			seen++
		}
		return seen
	}
	client, _, _ := newTestClient(t, ok(`{"items":[{"id":"a:1"}],"next_cursor":null,"count":1}`))
	if got := count(client); got != 1 {
		t.Errorf("got %d", got)
	}
	client, _, _ = newTestClient(t, ok(`{"items":[],"next_cursor":"x","count":0}`))
	if got := count(client); got != 0 {
		t.Errorf("got %d", got)
	}
}

func TestIterationStopsWhenTheCallerBreaks(t *testing.T) {
	client, api, _ := newTestClient(t, ok(`{"items":[{"id":"a:1"},{"id":"a:2"}],"next_cursor":"more","count":9}`))
	for range client.IterCars(context.Background(), MarketKR, nil, 0) {
		break
	}
	if len(api.requests) != 1 {
		t.Errorf("%d requests", len(api.requests))
	}
}

func TestIterationYieldsTheError(t *testing.T) {
	client, _, _ := newTestClient(t,
		ok(`{"items":[{"id":"a:1"}],"next_cursor":"more","count":9}`),
		answer{status: 402, body: `{"error":"quota_exhausted"}`},
	)
	var last error
	seen := 0
	for _, err := range client.IterCars(context.Background(), MarketKR, nil, 0) {
		seen++
		last = err
	}
	if seen != 2 || !errors.Is(last, ErrQuotaExceeded) {
		t.Errorf("seen = %d, err = %v", seen, last)
	}
}

func TestARepeatedCursorStopsTheIteration(t *testing.T) {
	page := ok(`{"items":[{"id":"a:1"}],"next_cursor":"SAME","count":9}`)
	client, api, _ := newTestClient(t, page, page)
	seen := 0
	var last error
	for _, err := range client.IterCars(context.Background(), MarketKR, nil, 0) {
		if err != nil {
			last = err
			continue
		}
		seen++
	}
	if !errors.Is(last, ErrCursorLoop) || seen != 2 || len(api.requests) != 2 {
		t.Errorf("seen = %d, requests = %d, err = %v", seen, len(api.requests), last)
	}
}

func TestACursorGivenAtTheStartCountsAsSeen(t *testing.T) {
	client, _, _ := newTestClient(t, ok(`{"items":[{"id":1,"car_id":"a:1"}],"next_cursor":"START"}`))
	var last error
	for _, err := range client.IterChanges(context.Background(), MarketKR, time.Now(), &ChangesParams{Cursor: "START"}, 0) {
		last = err
	}
	if !errors.Is(last, ErrCursorLoop) {
		t.Errorf("err = %v", last)
	}
}

func TestALargeCompressedAnswerIsCut(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	chunk := bytes.Repeat([]byte("A"), 1<<20)
	for range 70 {
		if _, err := writer.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		w.Write(compressed.Bytes())
	}))
	defer server.Close()
	client, err := NewClient(WithAPIKey(testKey), WithBaseURL(server.URL), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Usage(context.Background())
	var apiErr *Error
	if !errors.As(err, &apiErr) || !strings.Contains(apiErr.Message, "64 MiB") {
		t.Errorf("err = %v", err)
	}
	if errors.Is(err, ErrConnection) {
		t.Error("a large answer must not look like a network failure")
	}
}

func TestErrorTextIsCleanedAndShortened(t *testing.T) {
	detail := "a\x1b[31m\r\nb" + strings.Repeat("я", 900)
	body, _ := json.Marshal(map[string]string{"error": "bad\ncode", "detail": detail})
	client, _, _ := newTestClient(t, answer{status: 400, body: string(body)})
	_, err := client.Usage(context.Background())
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v", err)
	}
	if apiErr.Code != "bad code" {
		t.Errorf("code = %q", apiErr.Code)
	}
	if strings.ContainsFunc(apiErr.Message, unicode.IsControl) {
		t.Errorf("message has control characters: %.40q", apiErr.Message)
	}
	if got := len([]rune(apiErr.Detail)); got != 500 {
		t.Errorf("detail has %d characters", got)
	}
}

func TestRetryAfterIsCappedInThePublicField(t *testing.T) {
	want := func(n int) *int { return &n }
	for name, test := range map[string]struct {
		body   string
		header string
		want   *int
	}{
		"json negative":    {`{"retry_after":-5}`, "", want(0)},
		"json huge":        {`{"retry_after":9223372036854775807}`, "", want(60)},
		"json very huge":   {`{"retry_after":1e300}`, "", want(60)},
		"json float":       {`{"retry_after":1.5}`, "", want(2)},
		"json over":        {`{"retry_after":61}`, "", want(60)},
		"json wrong type":  {`{"retry_after":true}`, "", nil},
		"header negative":  {`{}`, "-5", want(0)},
		"header huge":      {`{}`, "99999999999999999999", want(60)},
		"header past":      {`{}`, "Wed, 21 Oct 2015 07:28:00 GMT", want(0)},
		"header far ahead": {`{}`, "Fri, 01 Jan 2100 00:00:00 GMT", want(60)},
		"header garbage":   {`{}`, "soon", nil},
		"header exponent":  {`{}`, "1e300", nil},
	} {
		headers := map[string]string{}
		if test.header != "" {
			headers["Retry-After"] = test.header
		}
		client, _, _ := newTestClient(t, answer{status: 429, headers: headers, body: test.body})
		client.maxRetries = 0
		_, err := client.Usage(context.Background())
		var apiErr *Error
		if !errors.As(err, &apiErr) {
			t.Fatalf("%s: err = %v", name, err)
		}
		switch {
		case test.want == nil && apiErr.RetryAfter != nil:
			t.Errorf("%s: got %d, want nil", name, *apiErr.RetryAfter)
		case test.want != nil && (apiErr.RetryAfter == nil || *apiErr.RetryAfter != *test.want):
			t.Errorf("%s: got %v, want %d", name, apiErr.RetryAfter, *test.want)
		}
	}
}

func TestNegativeMaxItemsIsAnError(t *testing.T) {
	client, api, _ := newTestClient(t)
	for _, err := range client.IterCars(context.Background(), MarketKR, nil, -1) {
		if err == nil {
			t.Error("no error")
		}
	}
	if len(api.requests) != 0 {
		t.Errorf("%d requests", len(api.requests))
	}
}

func TestChangesTakeATimeAndRepeatedTypes(t *testing.T) {
	client, api, _ := newTestClient(t, ok(fixture(t, "changes.json")))
	since := time.Date(2026, 11, 2, 12, 0, 0, 0, time.FixedZone("MSK", 3*3600))
	page, err := client.Changes(context.Background(), MarketCN, since, &ChangesParams{
		Types:  []ChangeType{ChangePriceDrop, ChangePriceUp},
		MinPct: 5,
		Limit:  5,
	})
	if err != nil {
		t.Fatal(err)
	}
	query := api.requests[0].URL.Query()
	if query.Get("since") != "2026-11-02T09:00:00Z" {
		t.Errorf("since = %s", query.Get("since"))
	}
	if strings.Join(query["type"], ",") != "price_drop,price_up" || query.Get("min_pct") != "5" {
		t.Errorf("query = %v", query)
	}
	event := page.Items[0]
	if event.Type != ChangePriceDrop || event.CarID != "che168:60161758" || event.OldPrice != "26000.00" ||
		event.NewPrice != "23000.00" || event.Pct != "11.538" || event.At.IsZero() {
		t.Errorf("event = %+v", event)
	}
	if _, err := client.Changes(context.Background(), MarketCN, time.Time{}, nil); err == nil {
		t.Error("no error for a zero since")
	}
}

func TestIterChangesFollowsTheCursor(t *testing.T) {
	client, api, _ := newTestClient(t,
		ok(`{"items":[{"id":1,"car_id":"iaai:1"}],"next_cursor":"c2"}`),
		ok(`{"items":[{"id":2,"car_id":"iaai:2"}],"next_cursor":null}`),
	)
	var ids []int64
	for event, err := range client.IterChanges(context.Background(), MarketUS, time.Now(), nil, 0) {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, event.ID)
	}
	if len(ids) != 2 || ids[1] != 2 || api.requests[1].URL.Query().Get("cursor") != "c2" {
		t.Errorf("ids = %v", ids)
	}
}

func TestReferencesAndUsage(t *testing.T) {
	client, api, _ := newTestClient(t,
		ok(`[{"name":"BYD","count":7015}]`),
		ok(`[{"name":"Song Pro","count":12}]`),
		ok(`{"fuel":{"gasoline":["petrol"]}}`),
		ok(`{"items":[{"market":"cn","source":"che168","available_in_plan":true,"status":"ok","data_updated_at":null,"data_age_seconds":12,"stale_after_seconds":7200,"stale":false,"active_cars":5}]}`),
		ok(`{"plan":"max","status":"active","expires_at":"2027-10-06T19:22:04Z","period":"2026-11","reset_at":"2026-12-01T00:00:00Z","quota_unit":"records","quota_limit":100,"quota_used":1,"by_market":[{"market":"cn","records":1,"requests":1,"live_us_requests":0}]}`),
	)
	ctx := context.Background()
	brands, err := client.Brands(ctx, MarketCN)
	if err != nil || brands[0].Count != 7015 {
		t.Errorf("brands = %v, %v", brands, err)
	}
	models, err := client.Models(ctx, MarketCN, "BYD")
	if err != nil || models[0].Name != "Song Pro" || api.requests[1].URL.Query().Get("brand") != "BYD" {
		t.Errorf("models = %v, %v", models, err)
	}
	dictionaries, err := client.Dictionaries(ctx)
	if err != nil || dictionaries["fuel"]["gasoline"][0] != "petrol" {
		t.Errorf("dictionaries = %v, %v", dictionaries, err)
	}
	markets, err := client.Markets(ctx)
	if err != nil || markets[0].DataAgeSeconds == nil || *markets[0].DataAgeSeconds != 12 || markets[0].DataUpdatedAt != nil {
		t.Errorf("markets = %+v, %v", markets, err)
	}
	usage, err := client.Usage(ctx)
	if err != nil || usage.Plan != "max" || usage.ByMarket[0].Market != MarketCN || usage.ExpiresAt.IsZero() {
		t.Errorf("usage = %+v, %v", usage, err)
	}
}

func TestUSLiveOnlyAsksWhenCalled(t *testing.T) {
	client, api, _ := newTestClient(t, ok(`[{"id":"copart:1","source_mode":"live"}]`))
	lots, err := client.USLive(context.Background(), &USLiveParams{Make: "Toyota", Page: 2})
	if err != nil || lots[0].SourceMode != "live" {
		t.Errorf("lots = %v, %v", lots, err)
	}
	if got := api.requests[0].RequestURI; got != "/v1/us/live?make=Toyota&page=2" {
		t.Errorf("uri = %s", got)
	}
}

func TestClientErrorsAreTypedAndNotRetried(t *testing.T) {
	for status, sentinel := range map[int]error{
		400: ErrBadRequest,
		401: ErrAuthentication,
		402: ErrQuotaExceeded,
		403: ErrPermissionDenied,
		404: ErrNotFound,
	} {
		client, api, waits := newTestClient(t, answer{
			status: status,
			body:   `{"error":"some_code","detail":"why","reset_at":"2026-12-01T00:00:00Z"}`,
		})
		_, err := client.Usage(context.Background())
		if !errors.Is(err, sentinel) {
			t.Errorf("%d: err = %v", status, err)
		}
		if errors.Is(err, ErrServer) || errors.Is(err, ErrConnection) {
			t.Errorf("%d matches too much: %v", status, err)
		}
		var apiErr *Error
		if !errors.As(err, &apiErr) {
			t.Fatalf("%d: not an *Error: %v", status, err)
		}
		if apiErr.StatusCode != status || apiErr.Code != "some_code" || apiErr.Detail != "why" || apiErr.ResetAt != "2026-12-01T00:00:00Z" {
			t.Errorf("%d: %+v", status, apiErr)
		}
		if want := fmt.Sprintf("apiauto: HTTP %d some_code: why", status); apiErr.Error() != want {
			t.Errorf("message = %q", apiErr.Error())
		}
		if len(api.requests) != 1 || len(*waits) != 0 {
			t.Errorf("%d: %d requests, %d waits", status, len(api.requests), len(*waits))
		}
	}
}

func TestRateLimitWaitsRetryAfterFromTheBody(t *testing.T) {
	client, _, waits := newTestClient(t,
		answer{status: 429, body: `{"error":"rate_limited","retry_after":3}`},
		ok(`{"plan":"max"}`),
	)
	usage, err := client.Usage(context.Background())
	if err != nil || usage.Plan != "max" {
		t.Fatalf("usage = %v, %v", usage, err)
	}
	if len(*waits) != 1 || (*waits)[0] != 3*time.Second {
		t.Errorf("waits = %v", *waits)
	}
}

func TestServiceUnavailableWaitsTheHeaderAndIsCapped(t *testing.T) {
	client, _, waits := newTestClient(t,
		answer{status: 503, headers: map[string]string{"Retry-After": "3600"}, body: `{"error":"upstream_quota_exhausted"}`},
		ok(`{"plan":"max"}`),
	)
	if _, err := client.Usage(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(*waits) != 1 || (*waits)[0] != 60*time.Second {
		t.Errorf("waits = %v", *waits)
	}
}

func TestRetryAfterAsAnHTTPDate(t *testing.T) {
	future := time.Now().Add(30 * time.Second).UTC().Format(http.TimeFormat)
	got := parseRetryAfter(future)
	if got == nil || *got < 25 || *got > 30 {
		t.Errorf("got = %v", got)
	}
	if parseRetryAfter("soon") != nil || parseRetryAfter("") != nil {
		t.Error("garbage must give nil")
	}
	if got := parseRetryAfter("-5"); got == nil || *got != 0 {
		t.Errorf("negative: %v", got)
	}
}

func TestServerErrorsBackOffAndGiveUp(t *testing.T) {
	client, api, waits := newTestClient(t,
		answer{status: 500, body: "oops"},
		answer{status: 502, body: "<html>bad gateway</html>"},
		answer{status: 500, body: "oops"},
	)
	_, err := client.Usage(context.Background())
	if !errors.Is(err, ErrServer) {
		t.Fatalf("err = %v", err)
	}
	var apiErr *Error
	errors.As(err, &apiErr)
	if apiErr.StatusCode != 500 || apiErr.Body != "oops" || apiErr.Message != "HTTP 500: oops" {
		t.Errorf("%+v", apiErr)
	}
	if len(api.requests) != 3 || len(*waits) != 2 {
		t.Fatalf("%d requests, waits %v", len(api.requests), *waits)
	}
	if (*waits)[0] < 250*time.Millisecond || (*waits)[0] > 500*time.Millisecond ||
		(*waits)[1] < 500*time.Millisecond || (*waits)[1] > time.Second {
		t.Errorf("waits = %v", *waits)
	}
}

func TestBackoffIsCapped(t *testing.T) {
	for attempt := 0; attempt < 12; attempt++ {
		if wait := backoff(attempt); wait > maxBackoff || wait <= 0 {
			t.Errorf("attempt %d: %v", attempt, wait)
		}
	}
}

func TestServiceUnavailableAlsoMatchesServer(t *testing.T) {
	client, _, _ := newTestClient(t, answer{status: 503, body: `{"error":"upstream_unavailable"}`})
	client.maxRetries = 0
	_, err := client.Usage(context.Background())
	if !errors.Is(err, ErrServiceUnavailable) || !errors.Is(err, ErrServer) {
		t.Errorf("err = %v", err)
	}
}

func TestConnectionErrorsAreRetried(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	address := server.URL
	server.Close() // nothing listens on this address any more
	client, err := NewClient(WithAPIKey(testKey), WithBaseURL(address))
	if err != nil {
		t.Fatal(err)
	}
	var waits atomic.Int32
	client.sleep = func(context.Context, time.Duration) error {
		waits.Add(1)
		return nil
	}
	_, err = client.Usage(context.Background())
	if !errors.Is(err, ErrConnection) {
		t.Fatalf("err = %v", err)
	}
	if waits.Load() != 2 {
		t.Errorf("waits = %d", waits.Load())
	}
	if strings.Contains(err.Error(), testKey) {
		t.Error("the error shows the key")
	}
}

func TestTimeoutIsPerAttemptAndReported(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer server.Close()
	defer close(release)
	client, err := NewClient(WithAPIKey(testKey), WithBaseURL(server.URL), WithTimeout(20*time.Millisecond), WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Usage(context.Background())
	if !errors.Is(err, ErrConnection) || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("err = %v", err)
	}
}

func TestACancelledContextStopsRetries(t *testing.T) {
	client, api, waits := newTestClient(t, answer{status: 500, body: "oops"}, answer{status: 500, body: "oops"})
	ctx, cancel := context.WithCancel(context.Background())
	client.sleep = func(context.Context, time.Duration) error {
		cancel()
		*waits = append(*waits, 0)
		return context.Canceled
	}
	if _, err := client.Usage(ctx); !errors.Is(err, ErrServer) {
		t.Errorf("err = %v", err)
	}
	if len(api.requests) != 1 {
		t.Errorf("%d requests", len(api.requests))
	}
}

func TestSleepContextEndsEarlyOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepContext(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
}

func TestASuccessThatIsNotJSONIsAnError(t *testing.T) {
	client, api, _ := newTestClient(t, ok("<html>"))
	_, err := client.Usage(context.Background())
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 200 || apiErr.Body != "<html>" {
		t.Errorf("err = %v", err)
	}
	if len(api.requests) != 1 {
		t.Errorf("%d requests", len(api.requests))
	}
}

func TestRedirectsAreNotFollowedSoTheKeyStaysHome(t *testing.T) {
	var hits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hits.Add(1)
	}))
	defer other.Close()
	yours := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return nil }}
	for _, status := range []int{301, 302, 303, 307, 308} {
		for name, options := range map[string][]Option{"default": nil, "your client": {WithHTTPClient(yours)}} {
			api := &fakeAPI{t: t, answers: []answer{{status: status, headers: map[string]string{"Location": other.URL}}}}
			server := httptest.NewServer(api)
			client, err := NewClient(append(options, WithAPIKey(testKey), WithBaseURL(server.URL))...)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Usage(context.Background())
			server.Close()
			var apiErr *Error
			if !errors.As(err, &apiErr) || apiErr.StatusCode != status {
				t.Errorf("%d, %s: err = %v", status, name, err)
			}
		}
	}
	if hits.Load() != 0 {
		t.Errorf("%d requests reached the other host", hits.Load())
	}
}

func TestYourHTTPClientIsCopied(t *testing.T) {
	yours := &http.Client{}
	client, err := NewClient(WithAPIKey(testKey), WithHTTPClient(yours))
	if err != nil {
		t.Fatal(err)
	}
	if yours.CheckRedirect != nil {
		t.Error("your client was changed")
	}
	if client.http.CheckRedirect == nil {
		t.Error("the copy still follows redirects")
	}
}

func TestLastResponseReadsTheHeaders(t *testing.T) {
	client, _, _ := newTestClient(t, answer{
		status: 200,
		body:   `{"items":[],"next_cursor":null,"count":0}`,
		headers: map[string]string{
			"X-Quota-Limit":         "100",
			"X-Quota-Remaining":     "97",
			"X-Quota-Reset":         "2026-10-28T12:04:00Z",
			"X-RateLimit-Limit":     "1",
			"X-RateLimit-Remaining": "0",
			"X-RateLimit-Reset":     "1",
			"X-Data-Age-Seconds":    "938",
		},
	})
	if client.LastResponse() != nil {
		t.Error("a response before the first request")
	}
	if _, err := client.Cars(context.Background(), MarketCN, nil); err != nil {
		t.Fatal(err)
	}
	info := client.LastResponse()
	if info.Status != 200 || *info.QuotaLimit != 100 || *info.QuotaRemaining != 97 || info.QuotaReset != "2026-10-28T12:04:00Z" ||
		*info.RateLimit != 1 || *info.RateRemaining != 0 || *info.RateReset != 1 || *info.DataAgeSeconds != 938 {
		t.Errorf("info = %+v", info)
	}
}

func TestAMissingHeaderIsNil(t *testing.T) {
	client, _, _ := newTestClient(t, ok(`{"plan":"max"}`))
	if _, err := client.Usage(context.Background()); err != nil {
		t.Fatal(err)
	}
	if client.LastResponse().QuotaLimit != nil {
		t.Error("expected nil")
	}
}
