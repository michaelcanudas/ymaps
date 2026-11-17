# Workflow

## Commits

One subject line. No body. No trailers. No co-author.

```
<type>(<scope>): <imperative summary>
```

Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`.

Scopes when the change has a home: `backend`, `frontend`, `contract`. Omit the scope when the change is the whole repo.

```
feat(backend): add hello response
feat(contract): add the hello operation
docs: define the two product constraints
```

Do not write `Added stuff`, a paragraph under the subject, `Co-authored-by`, or `Signed-off-by`.

## Pull requests

The title is the same subject line. The body is empty.

## Contract changes

The OpenAPI edit and the code that implements it land together. A client change that needs a new field waits until that field is in the file.
