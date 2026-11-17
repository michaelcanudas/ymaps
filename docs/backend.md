# Backend

Michael owns this directory. Walter's agents leave it alone unless a task explicitly tells them to change the server to match a contract edit they are landing in the same change.

## Stack

Go, standard library `net/http`. Add a dependency only when the standard library cannot do the job.

Layout:

- `cmd/ymaps` starts the process
- `internal/server` is the HTTP surface

## Rules

- Every product route is declared in `contract/openapi.yaml` and implemented here.
- Handlers return the schema in that file.
- Keep the route decision, the lexicon, prediction, and rerouting on the server once those operations exist.
- No comments. A name that needs a comment gets a better name.
