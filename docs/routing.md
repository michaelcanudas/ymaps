# Routing

How the backend turns a destination into a route. This is the backend's design, not an interface. The shapes under "What a route carries" are a proposal until they land in `contract/openapi.yaml`.

## Flow

1. A request carries the student's position and the destination as they said or typed it.
2. Place resolution turns the destination into a place with coordinates and, when known, an entryway.
3. The planner reads the latest snapshot of every mode, builds candidate routes, scores them, and returns the best one.
4. The client checks the route again while the student travels. The backend scores it again from where the student is now and returns a better route if one has appeared.

## Parts

Each part sits behind an interface. A part never imports another part's internals, and the planner never imports a provider.

```
place      destination text → candidate places
provider   data source      → timestamped snapshot
mode       snapshot + trip  → candidate legs
streets    points           → walk and ride times, paths
plan       legs             → scored route
api        HTTP             → contract
```

```go
type Resolver interface {
	Resolve(ctx context.Context, query string, near LatLon) ([]Candidate, error)
}

type Provider interface {
	Poll(ctx context.Context) (Snapshot, error)
}

type Mode interface {
	Legs(ctx context.Context, trip Trip, world World) []Leg
}

type Streets interface {
	Matrix(ctx context.Context, from, to []LatLon, profile Profile) ([][]time.Duration, error)
	Path(ctx context.Context, from, to LatLon, profile Profile) (Polyline, error)
}
```

Adding a place source is a new `Resolver`. Adding a data source is a new `Provider`. Adding a mode is a new `Mode`. Nothing else changes.

## Place resolution

Resolvers run in order. Their candidates are merged and ranked by confidence and distance from the student.

| Order | Resolver | Answers |
| --- | --- | --- |
| 1 | Aliases | Our own terms: `silliman buttery`, crowdsourced entryways |
| 2 | Yale map | Buildings, abbreviations like `WLH`, course locations like `ACCT 270`, entrances, dining halls. See [yale-map.md](integrations/yale-map.md). |
| 3 | Google Places | Any store, address, or landmark |

Aliases start as a file in the backend and later become stored, crowdsourced data. A confident early answer stops the chain. When no candidate is clearly best, the response asks the student to choose.

Interpreting free speech ("the place by Commons with the good bagels") uses Grok, per `docs/product.md`.

## Providers and snapshots

One poller per data source. Each writes an immutable snapshot with the time it was fetched, and the planner swaps to the new one atomically. Each source has its own freshness rule. A stale or failing source turns off only the modes that depend on it.

| Provider | Feeds | Modes | Doc |
| --- | --- | --- | --- |
| Downtowner | JSON | shuttle | [yale-shuttle.md](integrations/yale-shuttle.md) |
| GBFS reader | Ride New Haven, Veo | bike, veo | [ride-new-haven.md](integrations/ride-new-haven.md), [veo.md](integrations/veo.md) |
| GTFS reader | CTtransit static and realtime | bus | [cttransit.md](integrations/cttransit.md) |

The GBFS reader is shared. Per provider it takes the discovery URL, headers, and which feeds are expected. It reads feed URLs from `gbfs.json`, decodes every feed as GBFS 2.x whatever `version` says, and treats a `404` on an optional feed as absent.

## Streets

Walking, biking, and scooter times and paths come from a self-hosted [Valhalla](https://github.com/valhalla/valhalla) on OpenStreetMap data for New Haven. Valhalla's `pedestrian` and `bicycle` costing share one graph, and `bicycle` takes a speed per request, which covers bikes and scooters. Its `motor_scooter` costing is for mopeds; do not use it for Veo.

Origins and destinations are any point. Per request, one matrix call covers origin to nearby stops and stations, nearby stops and stations to destination, and origin to destination on foot. Walks between stops and stations are computed when the stop list changes and kept.

## Planning

Candidate routes come from a few shapes:

```
walk
walk → shuttle → walk
walk → bus → walk
walk → bike → walk
walk → veo → walk
walk → shuttle → walk → shuttle → walk
```

Each mode fills in its legs from the current snapshot. A mode with no fresh data contributes nothing.

Each leg carries a p50 and a p90 time, so the route has both. Routes are ranked on a late percentile of arrival plus a weight on cost, so a route that is usually fast but sometimes very late loses to a steady one. Uncertainty comes from:

- Shuttle and bus: predicted arrival error, measured from our own logs once they exist
- Bike and Veo: the chance the vehicle is gone by the time the student walks to it, and time to find and unlock it

If trips across town with several bus transfers start to matter, the shapes give way to a general transit search such as RAPTOR. Modes, providers, and streets stay as they are.

## Rerouting

The route has an id. When the client checks it again, the planner scores the legs still ahead from the student's current position, and also plans fresh from there. If the fresh route arrives meaningfully earlier at the late percentile, it replaces the old one.

## What a route carries

Proposed. The client draws its own map; the backend sends shapes, never images.

- The resolved place: name, coordinates, entryway
- Departure and arrival as absolute times, p50 and p90
- Total cost
- Legs, a union keyed by mode. Every leg has `from`, `to`, `departAt`, `arriveAt`, `cost`, and `path`, an encoded polyline of just that leg.
- Shuttle and bus legs add the line's name and color, board and alight stops, the vehicle's live position, and `line.path`, the whole route, drawn faintly behind the leg.
- Bike and Veo legs add the pickup and drop-off spot, and for Veo the app link for that vehicle.

Only lines the route uses are drawn. The full network map, as the shuttle tracker shows it, makes the student read a map and fails the fastest-time constraint.

All five modes go into the contract at once, even before the backend produces each one, so adding a mode later is not a breaking change.
