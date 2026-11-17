# Agents

Read this before editing anything. Then read [docs/product.md](docs/product.md), [docs/architecture.md](docs/architecture.md), [docs/workflow.md](docs/workflow.md), and, for backend work, [docs/routing.md](docs/routing.md).

## Who owns what

- Michael owns `backend/`. Walter's agents do not modify it.
- Walter owns `frontend/`. Michael's agents do not modify it.
- `contract/openapi.yaml` is the only interface between the two. Both sides may edit it. A behavior change in either side lands in that file in the same change.

## What to do

- Build only what serves the two product constraints in `docs/product.md`.
- Call and implement only operations that exist in `contract/openapi.yaml`.
- Keep code free of comments. Names carry the meaning.
- Use the commit rules in `docs/workflow.md`.

## What to leave alone

- Do not add a second API, a mock server with invented routes, or client-side routing logic that the contract does not express.
- Do not add an AI provider other than a Grok product.
- Do not add a feature that fails the product test in `docs/product.md`.
