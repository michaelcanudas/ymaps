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
mode       snapshot + query → rides and rentals
streets    points           → walk and ride times, paths
plan       network          → chosen route
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
	Network(ctx context.Context, query Query) (Network, error)
}

type Streets interface {
	Matrix(ctx context.Context, from, to []LatLon, profile Profile) ([][]time.Duration, error)
	Route(ctx context.Context, from, to LatLon, profile Profile) (Trace, error)
}
```

Adding a place source is a new `Resolver`. Adding a data source is a new `Provider`. Adding a mode is a new `Mode`. Nothing else changes.

## Place resolution

Resolvers run in order. Their candidates are merged and ranked by confidence and distance from the student.

| Order | Resolver | Answers |
| --- | --- | --- |
| 1 | Aliases | Our own terms: `silliman buttery`, `train station`, crowdsourced entryways. An alias names one building or place, never an area that would shadow the buildings inside it. |
| 1b | Courses | `ACCT 270`, `acct2700`, `CPSC 223`: course code to the building where it last met. See below. |
| 2 | Yale map | Buildings, abbreviations like `WLH`, course locations like `ACCT 270`, entrances, dining halls. See [yale-map.md](integrations/yale-map.md). |
| 3 | Google Places | Any store, address, or landmark |

Candidates from different resolvers that name the same place within 150 m count as one. Google results at the same spot are merged under the most specific name, and a station Google names after its city ("New Haven") becomes "New Haven Station".

### Courses

Yale stopped publishing meeting rooms after Spring 2025, and CourseTable's public API has none. The course table in `backend/internal/place/course/courses.json` comes from CourseTable's ferry data for Spring 2025, Fall 2024, Spring 2024, and Fall 2023, newest first: course code to Concept3D location id or building code. Codes accept the old three-digit and new four-digit numbers (`ACCT 270` and `ACCT 2700`). A course not in the table falls back to the building where most of its subject met, at lower confidence. Rooms may have moved since; refresh the table when a newer source has rooms.

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

Origins and destinations are any point. Per request, one matrix call covers the origin to the destination and to every stop and pickup within 1.7 km, and another covers every stop and drop-off within 1.7 km to the destination. Walks between stops, and rental ride times during the search, are estimated from distance; the chosen routes are then traced on the street graph.

## Planning

Modes do not build routes. Each mode turns its snapshot into a network for the request, and one search builds the route from all of them, so mixed routes such as shuttle → bus, bike → shuttle, or shuttle → shuttle come out of the same search as single-mode ones.

```go
type Mode interface {
	Network(ctx context.Context, query Query) (Network, error)
}

type Network struct {
	Rides   []Ride
	Rentals []Rental
}
```

- A `Ride` is one vehicle trip: a shuttle bus over its next two loops, or a CTtransit trip. It lists stops with arrival and departure times, a fare, and the line. Rides that share a fare group, such as CTtransit buses, pay once.
- A `Rental` is a set of pickups (a station with bikes, a free bike, a Veo vehicle) and drop-offs (a station, a Veo spot). Each pickup carries its speed, range, unlock and parking time, and price.
- Walking connects any two points: the student to nearby stops and pickups, stops and drop-offs to the destination, and stops to stops within 350 m.

The query covers points within 2.5 km of the straight line between the student and the destination.

### Search

The search is round-based and multi-criteria (McRAPTOR). Round *k* finds routes with *k* rides. Each label at a stop records arrival, walking time, outdoor waiting, cost, and fares paid, and a label survives only if no other label is at least as good on all of them. A round scans every ride from the stops reached in the round before, then every rental from pickups reached on foot, then walks from where those rides ended. Walking to the first stop is counted as walking, but waiting for the first ride is not, because the student leaves later and waits indoors; waiting at a transfer is outdoor waiting.

Rentals have rules of their own:

- Walking to a pickup takes at most 5 minutes.
- The pickup is never farther from the destination than where the walk to it started.
- The ride is at least 400 m and ends closer to the destination.
- A route has at most one rental.

Transit rides shorter than 3 minutes are skipped, and a route whose rides add up to less than a quarter of its time is not offered. The search keeps the best few routes, then each is filled in with real street paths and times and checked again.

### Choosing

There is no exchange rate between minutes and money. The planner picks a route by these steps:

1. A walk of 8 minutes or less wins.
2. Otherwise, take the earliest estimated arrival. Routes within 3 minutes of it, 5 at night, count as tied.
3. Among tied routes, prefer less burden: walking minutes, walking beyond 10 minutes counted again, outdoor waiting at 1.5 times, and 3 minutes per transfer. At night walking counts 1.5 times and waiting 2.5 times.
4. A free route within 4 burden-minutes of the lightest (6 at night) wins over a paid one. Otherwise the lightest wins, then the cheapest.

Night is after dusk and before dawn in New Haven, by month.

A request may carry `preferences`: most walking minutes, most cost in cents (0 for free only), and modes to avoid. They are hard limits. When no route meets them, the default route is returned.

### Estimates

Every time is an estimate; there is no late percentile.

- Shuttle: Downtowner ETAs for each bus, corrected by the error we measure (below), with stops between ETAs interpolated along the line's path. A request fetches fresh ETAs for the stops within 900 m of both ends when the cached ones are older than 20 seconds.
- Bus: the GTFS schedule plus CTtransit trip updates.
- Bike: 13 km/h and 2 minutes to unlock and dock.
- Veo: 13 km/h for scooters, 15 for e-bikes, and 3 minutes to unlock and park.

### Shuttle accuracy

Every ETA poll records the predicted arrival per bus and stop. When a bus's `lastStop` changes, the predictions for that stop are scored against the time the bus passed it, bucketed by how far ahead they were made. The correction per line and bucket starts at a measured prior and moves to the running mean as observations arrive.

The prior comes from 24 minutes of logged night service (189 scored predictions, Nov 17 2026): `lastStop` flips about when the bus leaves the stop, `avg` behaves like whole minutes rounded down, and buses passed 45 s later than predicted at under 3 minutes ahead, 30 s later at 3–5, and 20 s earlier at 6–10. Further out the error spread (±5 minutes) hides any bias, so the prior is 0. Blue Night ran about 100 s late at every horizon while the Orange lines ran early, which is why the learned correction is per line. The spread is wider than any correction, so the estimate stays an estimate.

## Rerouting

The route has an id. When the client checks it again, the planner plans fresh from the student's current position with the same place and preferences.

## What a route carries

Proposed. The client draws its own map; the backend sends shapes, never images.

- The resolved place: name, coordinates, entryway
- Departure and estimated arrival as absolute times
- Total cost
- Legs, a union keyed by mode. Every leg has `from`, `to`, `departAt`, `arriveAt`, `cost`, and `path`, an encoded polyline of just that leg.
- Shuttle and bus legs add the line's name and color, board and alight stops, the vehicle's live position, and `line.path`, the whole route, drawn faintly behind the leg.
- Bike and Veo legs add the pickup and drop-off spot, and for Veo the app link for that vehicle.

Only lines the route uses are drawn. The full network map, as the shuttle tracker shows it, makes the student read a map and fails the fastest-time constraint.

All five modes go into the contract at once, even before the backend produces each one, so adding a mode later is not a breaking change.
