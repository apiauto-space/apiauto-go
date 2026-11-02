# Contributing

Bug reports and pull requests are welcome at
[github.com/apiauto-space/apiauto-go](https://github.com/apiauto-space/apiauto-go/issues).
For questions about the API itself, data or plans, write to
[@apiautospace_bot](https://t.me/apiautospace_bot).

## Development

Before sending a pull request, run the same checks as CI:

```bash
gofmt -l .
go vet ./...
go test -race -count=1 ./...
```

Unit tests use `net/http/httptest` and never leave the machine. The `TestLive` tests in
`live_test.go` run against the real API only when `APIAUTO_KEY` is set and spend at most 10 records.

Keep the client thin: it mirrors the API and adds no logic of its own beyond pagination, retries
and error mapping. It uses only the standard library. Add a test for every change and a line to
`CHANGELOG.md`.
