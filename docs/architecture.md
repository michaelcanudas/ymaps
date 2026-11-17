# Architecture

```
ymaps/
  contract/openapi.yaml    the interface
  backend/                 Michael, Go
  frontend/                Walter
  docs/                    product, split, workflow
```

The backend is a Go HTTP server. The frontend is whatever Walter ships on top of the contract. The working assumption for the phone is React Native, plus Siri, a widget, and the top black bar. That assumption does not let the client grow a private API.

## The contract

`contract/openapi.yaml` is the source of truth.

- The server implements every operation in the file, and no other product operation.
- Every client calls operations from the file, and no others.
- Request and response bodies match the schemas. Unknown fields are not part of the contract.
- New behavior is an edit to the file plus the implementation. Shipping one without the other breaks the split.
- A breaking change uses a new path version. The current version is `/v1`.

Local server: `http://localhost:8080`. Set `PORT` to move it.

Browsers may send an `OPTIONS` preflight. The server answers `204` so a local client can call the API. Preflight is not a product operation and does not belong in the contract.

## Runtime

Go module: `ymaps/backend`.

```sh
cd backend/valhalla && docker compose up -d
cd backend && go run ./cmd/ymaps
cd backend && go test ./...
```

The first Valhalla start downloads the Connecticut extract and builds tiles, about five minutes. Later starts reuse `backend/valhalla/custom_files/`.

The server reads `.env` at the repo root.

| Variable | Needed for | Without it |
| --- | --- | --- |
| `RIDE_NEW_HAVEN_GBFS_KEY` | bike legs | no bike legs |
| `YALE_MAP_KEY` | Yale buildings, abbreviations, course locations, entrances | only our own aliases resolve |
| `GOOGLE_MAPS_KEY` | stores, addresses, anything off campus | those destinations return `422` |
| `VALHALLA_URL` | street paths and times, e.g. `http://localhost:8002` | straight lines at an estimated detour |
| `PORT` | listen port | `8080` |

The operations are `GET /v1/hello`, `POST /v1/routes`, and `GET /v1/routes/{routeId}`. How the backend plans a route is in [routing.md](routing.md).

```sh
curl -s -X POST localhost:8080/v1/routes -H 'Content-Type: application/json' \
  -d '{"origin":{"lat":41.2987,"lon":-72.9293},"destination":"Kline Tower"}'
```
