# ymaps

ymaps gets a Yale student the optimal route in the fastest time.

Two people build it in this repo. Michael owns `backend/`. Walter owns `frontend/`. They meet only through `contract/openapi.yaml`.

## Start here

1. [AGENTS.md](AGENTS.md) if you are an agent
2. [docs/product.md](docs/product.md) for what the product is and what it refuses to be
3. [docs/architecture.md](docs/architecture.md) for the split and the contract
4. [docs/workflow.md](docs/workflow.md) for commits
5. [docs/backend.md](docs/backend.md) or [docs/frontend.md](docs/frontend.md) for your side

## Run the backend

```sh
cd backend
go run ./cmd/ymaps
```

`GET http://localhost:8080/v1/hello` returns `{"message":"hello"}`. Override the port with `PORT`.
