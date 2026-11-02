# Security policy

## Reporting a vulnerability

Please do not open a public issue for a security problem in this client or in the
apiauto.space API. Write to the support of the Telegram bot
[@apiautospace_bot](https://t.me/apiautospace_bot) instead and describe what you found and how
to reproduce it. If your API key leaked, report it the same way so that it can be replaced.

## Supported versions

Security fixes go into the latest tagged release.

## What the tests check

- The key does not appear when a client is printed with any `fmt` verb, as a pointer, a value or inside
  a struct, slice or map, or in `json.Marshal`, or in errors. A key a header cannot carry is refused
  without showing it.
- The client does not follow 302 redirects, including through your own `http.Client`, so the key does
  not reach another host.
- A plain `http` base URL is refused except for localhost; a base URL with a user, query or fragment
  is refused, and the error does not repeat it.
- An answer above 64 MiB is cut while reading, compressed answers included.
- `Retry-After` is kept between 0 and 60, in the `RetryAfter` field too. Iteration stops on a repeated
  cursor and on an empty page.
- The error code, detail and reset time have no control characters and at most 500 characters.

## What the tests do not check

- TLS: the client uses the standard library's checks, but no test connects to a host with a bad
  certificate. A `WithHTTPClient` client whose transport turns the checks off stays that way.
- Code inside your process can still read the key through a debugger or a memory dump. A transport or
  middleware of your own sees it in the request headers.
- `Error.Body` and all listing data are untrusted text from the API and its sources. Escape them before
  showing them in HTML.

Keep the key in the `APIAUTO_KEY` environment variable or a secret store, and call the API from your
server only: it does not accept browser requests.
