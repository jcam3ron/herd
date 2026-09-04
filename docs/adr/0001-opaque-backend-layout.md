# 1. Keep `backend.Layout` opaque

## Status

Accepted

## Context

`backend.Layout` (`internal/backend/backend.go`) is `json.RawMessage`: shared
code (package `herd`) stores and replays it without interpreting it, and only
the backend that produced it (`internal/backend/niri` today) knows its shape
(`{"col":int,"row":int}`).

With exactly one backend implemented, this is a hypothetical seam by the "one
adapter = hypothetical seam, two = real" rule: nothing today actually varies
across it. Deleting the opacity (a concrete `Layout {Col, Row int}` struct
shared by `backend` and `herd`) would delete a `json.Marshal` call and two
`json.Unmarshal` call sites in `niri.go` with no complexity reappearing
elsewhere.

## Decision

Keep `Layout` opaque, even though only one adapter exists.

This isn't the general case of a premature seam: the `backend` package doc
already states the intent of a second adapter ("others later, e.g. an omniwm
backend for Mac"), and a Mac window manager's layout descriptor won't share
niri's column/row scrolling model. Concretizing `Layout` now would mean
re-abstracting it the moment a second backend lands, for no benefit in between.

## Consequences

- `herd`'s shared code continues to treat `Layout` as opaque JSON; only the
  producing backend interprets it.
- Revisit this decision once a second `backend.Backend` adapter exists and its
  layout shape is known — if a shared subset of fields emerges, concretize then,
  not before.
