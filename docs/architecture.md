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
cd backend
go run ./cmd/ymaps
go test ./...
```

The current operation is `GET /v1/hello`.
