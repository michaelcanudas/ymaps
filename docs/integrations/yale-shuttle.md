# Yale Shuttle

The Yale Shuttle tracker at `https://yale.downtownerapp.com/routes` is a single-page app run by Downtowner. It reads live data from a small JSON API on the same host. This file documents that API so the backend can use it as the shuttle data source.

The API is not published or versioned by Downtowner. Everything here was read from the app's bundle and checked against live responses on 2026-11-17. Treat it as something that can change without notice.

## Who calls it

Only the backend. The API sends no `Access-Control-Allow-*` headers, so a browser on another origin cannot call it, and the frontend does not talk to mode providers anyway. Shuttle facts reach clients only through operations in `contract/openapi.yaml`.

## Basics

- Base URL: `https://yale.downtownerapp.com`
- Method: `GET` on every endpoint
- Auth: none. No key, token, or cookie is needed. The load balancer sets `AWSELB` cookies; they can be ignored.
- Format: `application/json; charset=utf-8`
- Errors: a bad or missing parameter returns `400` with the plain-text body `Bad Request`.
- Caching: responses carry an `ETag`. Send `If-None-Match` and a `304` comes back when nothing changed.
- Rate limit: responses carry `RateLimit: limit=3000;w=60`. Stay at the polling plan below anyway.
- Times: Unix seconds.
- Coordinates: WGS84 decimal degrees, as `lat` and `lon`.
- IDs: integers. A stop ID, route ID, and bus ID are separate spaces.

## Endpoints

| Endpoint | What it returns | Tracker refresh |
| --- | --- | --- |
| `GET /routes_routes.php` | Routes, their stops in order, and their path | 60 s |
| `GET /routes_stops.php` | Every stop with its location | once |
| `GET /routes_buses.php` | Every shuttle on the road right now | 5 s |
| `GET /routes_eta.php?stop={id}` | Predicted arrivals at one stop | 30 s |
| `GET /routes_announcements.php` | Service notices | once |
| `GET /client_config` | Branding and map settings for the tracker | once |

The refresh column is what the tracker itself uses. Polling faster than that gains nothing and puts load on a host we do not own.

### `GET /routes_routes.php`

Query: `inactive=true` also returns routes that are not running now. Without it, only active routes come back.

```json
[
  {
    "id": 1,
    "name": "Blue - Weekday Daytime",
    "short_name": "BD",
    "description": "7am - 6pm, M - F",
    "color": "4472C4",
    "active": true,
    "stops": [106, 34, 101, 47, 100],
    "path": [41.315763, -72.924481, 41.31606, -72.924399]
  }
]
```

| Field | Type | Meaning |
| --- | --- | --- |
| `id` | int | Route ID. Matches `route` on buses and ETAs. |
| `name` | string | Display name, including the service window. |
| `short_name` | string | Short code. Not unique: `BW` is both route 4 and route 16. |
| `description` | string | Service hours as free text. Not machine-readable. |
| `color` | string | Hex RGB without `#`. |
| `active` | bool | Whether the route is running now. |
| `stops` | int[] | Stop IDs in the order the shuttle serves them. The route is a loop. |
| `path` | number[] | Flat polyline: `lat, lon, lat, lon, …`. Pair the values; the even index is latitude. |

Routes on 2026-11-17, with `inactive=true`:

| ID | Name | Hours | Active then |
| --- | --- | --- | --- |
| 1 | Blue - Weekday Daytime | 7am - 6pm, M - F | yes |
| 2 | Orange - Weekday Daytime | 7am - 6pm, M - F | yes |
| 3 | Red - Weekday Daytime | 7am - 6pm, M - F | yes |
| 4 | Blue - Weekend Daytime | 7am - 6pm, Sat - Sun | no |
| 8 | Pink - VA Hospital / Med School | 6am - 6:30pm, M - F | yes |
| 9 | Green - West Campus | 5:30am - 6:40pm, Daily | yes |
| 10 | Purple - West Campus | 5:30am - 11:45pm, Daily | yes |
| 13 | Blue - Night | 6pm - 12am, Daily | yes |
| 14 | Orange - Night | 6pm - 12am, Daily | no |
| 15 | Gold Route | 8am - 6pm, M - F | yes |
| 16 | Blue West | 6pm - 12am, Daily | no |
| 17 | Orange East | 6pm - 12am, Daily | no |
| 18 | Weekend Grocery (to Hamden) | 7am - 5pm, Sat - Sun | no |
| 19 | Brown Connector | 6am - 7pm, Mon - Fri | yes |

### `GET /routes_stops.php`

No query parameters. Returns all 173 stops, whether or not a running route serves them.

```json
[
  { "id": 1, "name": "100 Church Street South", "description": "", "lat": 41.299671, "lon": -72.929425 },
  { "id": 3, "name": "130 Prospect Street (N)", "description": "", "lat": 41.31532, "lon": -72.92455 }
]
```

| Field | Type | Meaning |
| --- | --- | --- |
| `id` | int | Stop ID. Used by `stops` on routes, `lastStop` on buses, and `routes_eta.php`. |
| `name` | string | Display name. `(N)` and `(S)` mark the two sides of a street. |
| `description` | string | Empty for every stop today. |
| `lat`, `lon` | number | Stop location. |

A stop name is the closest thing the shuttle has to an entryway or alias. It is not a place. Mapping stops to places is backend work.

### `GET /routes_buses.php`

No query parameters. Returns every shuttle currently reporting. Returns `[]` when nothing is running.

```json
[
  {
    "id": 69251,
    "name": "#44",
    "lat": 41.313287,
    "lon": -72.925187,
    "heading": 11,
    "route": 2,
    "lastStop": 20,
    "lastUpdate": 1794955936
  }
]
```

| Field | Type | Meaning |
| --- | --- | --- |
| `id` | int | Bus ID. Matches `bus_id` in ETAs. |
| `name` | string | Vehicle number shown to riders. |
| `lat`, `lon` | number | Last reported position. |
| `heading` | int | Degrees clockwise from north. |
| `route` | int | Route the bus is serving now. |
| `lastStop` | int | Stop ID the bus last served. |
| `lastUpdate` | int | Unix seconds of the last GPS fix. Compare with now to drop stale buses. |

Positions update about every 5 seconds.

### `GET /routes_eta.php?stop={id}`

Query: `stop` is one stop ID, required.

```json
{
  "etas": {
    "20": {
      "etas": [
        { "avg": 51, "bus_id": 69251, "bus_name": "#44", "route": 2 },
        { "avg": 34, "bus_id": 69268, "bus_name": "#45", "route": 2 },
        { "avg": 8, "bus_id": 69269, "bus_name": "#43", "route": 1 }
      ]
    }
  },
  "calculation_time": 1794955950
}
```

| Field | Type | Meaning |
| --- | --- | --- |
| `etas` | object | Keyed by stop ID as a string. Holds one key. |
| `etas[stop].etas` | array | One entry per bus predicted to reach the stop. Not sorted. |
| `avg` | int | Minutes until that bus reaches the stop. The tracker shows `0` as "Arriving". |
| `bus_id` | int | Bus ID from `routes_buses.php`. |
| `bus_name` | string | Vehicle number. |
| `route` | int | Route the bus is serving. |
| `calculation_time` | int | Unix seconds when Downtowner computed the predictions. |

Behavior:

- An unknown stop ID returns `{}`.
- A missing or non-numeric `stop` returns `400 Bad Request`.
- Ask for one stop per request. `stop=20&stop=3` does not return both; it returned the predictions for stop `1` under the key `"1"`. The tracker fans out one request per stop.
- A bus that just left the stop shows a full loop ahead. Bus `#44` had `lastStop: 20` and `avg: 51` for stop 20.
- `avg` is Downtowner's prediction. It is a starting observation for our own shuttle prediction, not the answer.

### `GET /routes_announcements.php`

No query parameters. Returns service notices. The tracker treats anything other than a non-empty array as "no notices", so the backend should too.

```json
[
  {
    "id": 36,
    "title": "Red, Brown line",
    "message": "Starting Friday November 13, 2026 the Brown line and Red Line new stop will be at 100 Church street South. The Red Line stop at Amistad and Church has been removed.",
    "button_text": null,
    "button_url": null
  }
]
```

`title` names the affected lines as free text. Nothing links an announcement to a route ID or stop ID.

### `GET /client_config`

Tracker branding: title, logo, colors, sidebar links, the tracker's own Mapbox public token, and the initial map view (`41.312302, -72.926917`, zoom 14). It carries no shuttle data. The backend does not need it.

## Joining the data

```
routes_routes.php   route.id ─────┬──── bus.route ──── routes_buses.php
                    route.stops[] │         bus.id
                          │       │           │
routes_stops.php    stop.id ──────┼── bus.lastStop
                          │       │           │
routes_eta.php?stop=stop.id → eta.route, eta.bus_id
```

To answer "when does a shuttle reach stop B from stop A":

1. Load routes with `inactive=true` and stops once, then refresh routes every minute to pick up `active` changes.
2. Find routes whose `stops` contain both A and B. Because routes are loops, B can come before A in the array; the ride wraps around.
3. Call `routes_eta.php?stop=A` and keep entries whose `route` is one of those routes. The smallest `avg` is the next pickup.
4. Call `routes_eta.php?stop=B` and read the same `bus_id`. Its `avg` there minus its `avg` at A is the ride time. If the bus has no entry for B, the prediction does not reach that far. If the difference is negative, the bus reaches B before A and the rider needs a later pass.
5. Use `routes_buses.php` for live positions and to drop any bus whose `lastUpdate` is old.

## Quick check

```sh
curl -s https://yale.downtownerapp.com/routes_buses.php
curl -s "https://yale.downtownerapp.com/routes_eta.php?stop=20"
```

## Polling plan

| Endpoint | Interval | Notes |
| --- | --- | --- |
| `routes_routes.php?inactive=true` | 60 s | With `If-None-Match`. Recompute stop offsets only when it changes. |
| `routes_stops.php` | startup, then hourly | With `If-None-Match`. 173 stops, 33 on no route. |
| `routes_buses.php` | 5 s | |
| `routes_eta.php?stop={id}` | 30 s | Only stops on routes that have a live bus, staggered across the window. |
| `routes_announcements.php` | 10 min | Display text only. |

Each feed has its own freshness. The shuttle mode is unavailable when buses are older than 30 s or ETAs older than 90 s.

## Computing a shuttle leg

For each route with a live bus that serves board stop A and alight stop B:

1. For every index `i` where `stops[i] == A`, the ride length is `(j - i) mod n` stops for the nearest `j` with `stops[j] == B`. This handles the loop and stops listed twice.
2. For each bus on the route whose `lastUpdate` is under 60 s old, the wait is its ETA at A.
3. The ride time is that bus's ETA at B minus its ETA at A. If B has no entry for the bus, or the difference is not positive, use the median of recent observed differences for that route and stop pair, then path distance at 4.5 m/s plus 20 s per stop.
4. If the wait is shorter than the walk to A, the student misses that pass. Use the next one: add one loop time, which is close to the bus's ETA at its own `lastStop`.
5. The estimate is the Downtowner ETA plus the correction measured from our own arrivals. See "Shuttle accuracy" in [routing.md](../routing.md).

## Slicing the path for a leg

Stops sit up to about 70 m off `path`, and the first stop is not at path offset 0. Snapping each stop to its nearest path point fails: on route 14 one stop snapped to the far side of the street, 3.7 km from its true position along the loop.

What works:

1. For each stop, take projections onto every path segment within 75 m as candidates.
2. Choose one candidate per stop with a small dynamic program over the stop order that minimizes forward gaps along the loop plus half the snap distance.
3. Check that the gaps add up to exactly one loop. If they do not, mark the route's path untrusted.

On 2026-11-17 this gave exactly one loop for 13 of 14 routes, worst snap 68 m. Route 9 (Green, West Campus) came out at three loops because its stop order runs against the path direction. An untrusted route gets a straight-line leg geometry until it checks out again.

A leg's geometry is the path from the snapped point at A to the snapped point at B, wrapping at the end of the array.

## Our own prediction

`avg` is a starting observation. To beat it:

1. Log every bus poll. A bus arrives at a stop when `lastStop` changes to it or it comes within 30 m of the stop's snapped point.
2. Log every ETA poll.
3. Ride time: observed stop-to-stop times per route, segment, and hour of week.
4. Wait: compare logged ETAs with actual arrivals. The estimate is `avg` plus the mean error for that horizon.

Keep raw positions for 7 days. Keep arrivals and ETA errors.

## Live behavior not visible in the bundle

Seen on 2026-11-17 around 18:10 ET, during the day-to-night changeover:

- `active` follows the printed schedule, not the buses. Route 3 was still active with buses after its 6pm end; route 1 was already inactive. Trust a fresh bus or ETA over `active` and `description`.
- One stop can list ETAs from routes on both schedules at once. Stop 10 showed routes 10, 13, 14, 16, and 17.
- `lastStop` is `0` on a bus that has just started. Its ETAs are still usable.
- `short_name` is not always short. Route 19's is `Brown Connector`.
- Routes 9 and 10 list the same stop twice because they run out and back.
- `(N)` and `(S)` stops snap to the same path offset. Use the stop the route lists, never one picked by name.
- `avg` is whole minutes, so a ride time from two ETAs is off by up to a minute.
- Bus `lastUpdate` was 0 to 9 s old.
- Stop lists already reflect announcements. Route 3 now serves stops 1 and 115; stop 14 (Amistad and Church) appears only on inactive route 4. Announcements do not need parsing.
