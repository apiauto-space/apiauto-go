package apiauto

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// Version is the version of this library.
	Version = "0.1.0"
	// DefaultBaseURL is the API root.
	DefaultBaseURL = "https://api.apiauto.space/v1"
	// APIKeyEnv is the environment variable the key is read from.
	APIKeyEnv = "APIAUTO_KEY"

	demoKeyURL        = "https://t.me/apiautospace_bot?start=demo"
	defaultTimeout    = 30 * time.Second
	defaultMaxRetries = 2
	defaultPageSize   = 20
	maxBackoff        = 8 * time.Second
	// A page is at most 100 records: anything far above that is not an API answer.
	maxBodyBytes = 64 << 20
)

// Client talks to the apiauto.space API. Create it with [NewClient]; it is safe for concurrent use.
type Client struct {
	// key is a function so that fmt and reflection, which print any string field whatever the
	// verb, see a function value instead of the key.
	key        func() string
	baseURL    string
	timeout    time.Duration
	maxRetries int
	http       *http.Client
	sleep      func(context.Context, time.Duration) error
	state      *responseState
}

// responseState is shared by copies of a Client, so that a copy holds no lock.
type responseState struct {
	mu   sync.Mutex
	last *ResponseInfo
}

func keyFunc(key string) func() string {
	return func() string { return key }
}

// Option configures [NewClient].
type Option func(*Client) error

// WithAPIKey sets the key instead of the APIAUTO_KEY environment variable.
func WithAPIKey(key string) Option {
	return func(c *Client) error {
		c.key = keyFunc(strings.TrimSpace(key))
		return nil
	}
}

// WithBaseURL sets the API root: https, or http for localhost; no user, query or fragment.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) error {
		// The URL stays out of the errors: a user part in it may be a password.
		parsed, err := url.Parse(baseURL)
		if err != nil || parsed.Host == "" || parsed.User != nil || strings.ContainsAny(baseURL, "?#") {
			return errors.New("apiauto: the base URL must be a plain URL without a user, query or fragment")
		}
		host := parsed.Hostname()
		local := host == "localhost" || host == "127.0.0.1" || host == "::1"
		if parsed.Scheme != "https" && !(parsed.Scheme == "http" && local) {
			return errors.New("apiauto: the base URL must use https: the key travels in a header")
		}
		c.baseURL = strings.TrimRight(baseURL, "/")
		return nil
	}
}

// WithTimeout sets how long one attempt may take, 30 seconds by default.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) error {
		if timeout <= 0 {
			return errors.New("apiauto: the timeout must be above zero")
		}
		c.timeout = timeout
		return nil
	}
}

// WithMaxRetries sets how many times to retry after 429, 5xx or a network error, 2 by default.
// 429 and 503 wait Retry-After (at most 60 seconds), the rest back off exponentially. Other 4xx
// errors are never retried.
func WithMaxRetries(retries int) Option {
	return func(c *Client) error {
		if retries < 0 {
			return errors.New("apiauto: max retries must be 0 or more")
		}
		c.maxRetries = retries
		return nil
	}
}

// WithHTTPClient sets your own HTTP client (a proxy, a transport, tracing). Its redirect policy
// is replaced on a copy: the client never follows a redirect, because the key is in the headers.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) error {
		if httpClient == nil {
			return errors.New("apiauto: the HTTP client must not be nil")
		}
		copied := *httpClient
		c.http = &copied
		return nil
	}
}

// NewClient returns a client. Without [WithAPIKey] the key comes from APIAUTO_KEY.
func NewClient(options ...Option) (*Client, error) {
	c := &Client{
		key:        keyFunc(strings.TrimSpace(os.Getenv(APIKeyEnv))),
		baseURL:    DefaultBaseURL,
		timeout:    defaultTimeout,
		maxRetries: defaultMaxRetries,
		http:       &http.Client{},
		sleep:      sleepContext,
		state:      &responseState{},
	}
	for _, option := range options {
		if err := option(c); err != nil {
			return nil, err
		}
	}
	key := c.key()
	if key != "" && strings.IndexFunc(key, func(r rune) bool { return r < 0x21 || r > 0x7e }) >= 0 {
		return nil, errors.New("apiauto: the API key must be printable ASCII without spaces")
	}
	if key == "" {
		return nil, fmt.Errorf("apiauto: no API key: use WithAPIKey or set the %s environment variable; "+
			"a free demo key comes from %s", APIKeyEnv, demoKeyURL)
	}
	c.http.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return c, nil
}

// String describes the client without the key.
func (c *Client) String() string {
	return fmt.Sprintf("apiauto.Client{baseURL: %q, apiKey: \"***\"}", c.baseURL)
}

// GoString describes the client in %#v without the key.
func (c *Client) GoString() string { return c.String() }

// LastResponse returns the quota and rate headers of the last response, successful or not; nil
// before the first one.
func (c *Client) LastResponse() *ResponseInfo {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	return c.state.last
}

// Cars returns one page of a market's catalog: GET /cars. Every car returned counts against the
// quota. params may be nil.
func (c *Client) Cars(ctx context.Context, market Market, params *CarsParams) (*CarsPage, error) {
	query, err := carsQuery(market, params)
	if err != nil {
		return nil, err
	}
	var page CarsPage
	if err := c.get(ctx, "/cars", query, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// IterCars yields every car matching the filters, following next_cursor. With maxItems above 0 it
// stops after that many cars, and the last request asks only for what is still needed, so no quota
// is spent on extra records. The iteration ends at the first error, which it yields.
func (c *Client) IterCars(ctx context.Context, market Market, params *CarsParams, maxItems int) iter.Seq2[Car, error] {
	return func(yield func(Car, error) bool) {
		query, err := carsQuery(market, params)
		if err != nil {
			yield(Car{}, err)
			return
		}
		paginate(ctx, c, "/cars", query, maxItems, yield)
	}
}

// Car fetches one car by id, such as encar:43020893: GET /cars/{id}. Cars taken off sale stay
// available for 14 days with IsActive false.
func (c *Client) Car(ctx context.Context, id string) (*Car, error) {
	if strings.TrimSpace(id) == "" || id == "." || id == ".." {
		return nil, errors.New("apiauto: the car id must look like encar:43020893")
	}
	var car Car
	if err := c.get(ctx, "/cars/"+url.PathEscape(id), nil, &car); err != nil {
		return nil, err
	}
	return &car, nil
}

// Changes returns one page of the change feed since a moment: GET /changes. Each event counts as
// one record; the feed comes with the Pro, Max and demo plans. params may be nil.
func (c *Client) Changes(ctx context.Context, market Market, since time.Time, params *ChangesParams) (*ChangesPage, error) {
	query, err := changesQuery(market, since, params)
	if err != nil {
		return nil, err
	}
	var page ChangesPage
	if err := c.get(ctx, "/changes", query, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// IterChanges yields every event since a moment, following next_cursor; maxItems works as in
// [Client.IterCars].
func (c *Client) IterChanges(ctx context.Context, market Market, since time.Time, params *ChangesParams, maxItems int) iter.Seq2[Change, error] {
	return func(yield func(Change, error) bool) {
		query, err := changesQuery(market, since, params)
		if err != nil {
			yield(Change{}, err)
			return
		}
		paginate(ctx, c, "/changes", query, maxItems, yield)
	}
}

// Brands returns a market's makes with car counts, most frequent first. Free.
func (c *Client) Brands(ctx context.Context, market Market) ([]ReferenceItem, error) {
	if err := market.check(); err != nil {
		return nil, err
	}
	var items []ReferenceItem
	err := c.get(ctx, "/references/brands", url.Values{"market": {string(market)}}, &items)
	return items, err
}

// Models returns a make's models with car counts, most frequent first. Free.
func (c *Client) Models(ctx context.Context, market Market, brand string) ([]ReferenceItem, error) {
	if err := market.check(); err != nil {
		return nil, err
	}
	var items []ReferenceItem
	err := c.get(ctx, "/references/models", url.Values{"market": {string(market)}, "brand": {brand}}, &items)
	return items, err
}

// Dictionaries returns the canonical values of fuel, transmission, body_type and more, with
// synonyms: field name, then canonical value, then synonyms. Free.
func (c *Client) Dictionaries(ctx context.Context) (Dictionaries, error) {
	var result Dictionaries
	err := c.get(ctx, "/references/dictionaries", nil, &result)
	return result, err
}

// Markets returns every market's plan access, status, data age and active cars. Free.
func (c *Client) Markets(ctx context.Context) ([]MarketStatus, error) {
	var body struct {
		Items []MarketStatus `json:"items"`
	}
	err := c.get(ctx, "/references/markets", nil, &body)
	return body.Items, err
}

// Usage returns the plan, its term and how much of the quota is spent. Free.
func (c *Client) Usage(ctx context.Context) (*Usage, error) {
	var usage Usage
	if err := c.get(ctx, "/usage", nil, &usage); err != nil {
		return nil, err
	}
	return &usage, nil
}

// USLive searches Copart and IAAI lots live at the auction data provider: GET /us/live. It is for
// cars not in the catalog copy yet. Pro and Max plans; it spends a separate monthly counter, not
// the car quota. params may be nil.
func (c *Client) USLive(ctx context.Context, params *USLiveParams) ([]Car, error) {
	var cars []Car
	err := c.get(ctx, "/us/live", params.query(), &cars)
	return cars, err
}

// pageOf is the part of a page the iterators read.
type pageOf[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor"`
}

func paginate[T any](ctx context.Context, c *Client, path string, query url.Values, maxItems int, yield func(T, error) bool) {
	var zero T
	if maxItems < 0 {
		yield(zero, errors.New("apiauto: maxItems must be 0 or more"))
		return
	}
	pageSize := defaultPageSize
	if limit := query.Get("limit"); limit != "" {
		pageSize, _ = strconv.Atoi(limit)
	}
	remaining := maxItems
	seen := map[string]struct{}{}
	if cursor := query.Get("cursor"); cursor != "" {
		seen[cursor] = struct{}{}
	}
	for {
		if maxItems > 0 {
			query.Set("limit", strconv.Itoa(min(pageSize, remaining)))
		}
		var page pageOf[T]
		if err := c.get(ctx, path, query, &page); err != nil {
			yield(zero, err)
			return
		}
		for _, item := range page.Items {
			if !yield(item, nil) {
				return
			}
			if maxItems > 0 {
				if remaining--; remaining == 0 {
					return
				}
			}
		}
		if page.NextCursor == "" || len(page.Items) == 0 {
			return
		}
		if _, repeated := seen[page.NextCursor]; repeated {
			yield(zero, ErrCursorLoop)
			return
		}
		seen[page.NextCursor] = struct{}{}
		query.Set("cursor", page.NextCursor)
	}
}

func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	target := c.baseURL + path
	if encoded := query.Encode(); encoded != "" {
		target += "?" + encoded
	}
	for attempt := 0; ; attempt++ {
		err := c.attempt(ctx, target, out)
		if err == nil {
			return nil
		}
		var apiErr *Error
		if ctx.Err() != nil || !errors.As(err, &apiErr) {
			return err
		}
		delay, retry := c.retryDelay(apiErr, attempt)
		if !retry {
			return err
		}
		if waitErr := c.sleep(ctx, delay); waitErr != nil {
			return err
		}
	}
}

func (c *Client) attempt(ctx context.Context, target string, out any) error {
	attemptCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("apiauto: bad request: %w", err)
	}
	request.Header.Set("X-API-Key", c.key())
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "apiauto-go/"+Version+" (+https://github.com/apiauto-space/apiauto-go)")

	response, err := c.http.Do(request)
	if err != nil {
		return connectionError(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBodyBytes+1))
	if err != nil {
		return connectionError(err)
	}

	c.state.mu.Lock()
	c.state.last = responseInfoFrom(response)
	c.state.mu.Unlock()

	if len(body) > maxBodyBytes {
		return &Error{Message: "the API answer is larger than 64 MiB: stopped reading it", StatusCode: response.StatusCode}
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errorFromResponse(response.StatusCode, response.Header.Get("Retry-After"), body)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return &Error{
			Message:    "the API answered with a body that could not be read as JSON: " + err.Error(),
			StatusCode: response.StatusCode,
			Body:       string(body),
		}
	}
	return nil
}

func (c *Client) retryDelay(err *Error, attempt int) (time.Duration, bool) {
	if attempt >= c.maxRetries {
		return 0, false
	}
	switch {
	case (err.StatusCode == http.StatusTooManyRequests || err.StatusCode == http.StatusServiceUnavailable) && err.RetryAfter != nil:
		return time.Duration(*err.RetryAfter) * time.Second, true
	case err.StatusCode == http.StatusTooManyRequests || err.StatusCode >= 500 || err.StatusCode == 0:
		return backoff(attempt), true
	}
	return 0, false
}

// backoff is exponential with jitter: about 0.5 s, 1 s, 2 s ... up to 8 s.
func backoff(attempt int) time.Duration {
	wait := maxBackoff
	if attempt < 4 {
		wait = 500 * time.Millisecond << attempt
	}
	return time.Duration(float64(wait) * (0.5 + rand.Float64()/2))
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
