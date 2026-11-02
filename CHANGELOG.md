# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/).

## [0.1.0] — Unreleased

First release.

### Added

- `apiauto.Client` for the apiauto.space API: Encar (Korea), Che168 (China), Copart and IAAI (USA).
- Catalog search: `Cars`, `IterCars` with a limit of items, `Car`.
- Change feed: `Changes` and `IterChanges`.
- References and usage: `Brands`, `Models`, `Dictionaries`, `Markets`, `Usage`.
- Live US search: `USLive`.
- `*apiauto.Error` with sentinels for `errors.Is`.
- Retries on 429 and 503 after `Retry-After`, and on 5xx and network errors with backoff.
- Quota and rate-limit headers in `LastResponse`.
- Structs for every response.

[0.1.0]: https://github.com/apiauto-space/apiauto-go/releases/tag/v0.1.0
