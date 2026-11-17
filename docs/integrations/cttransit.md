# CTtransit

The New Haven bus. CTtransit publishes a static GTFS schedule and GTFS-realtime feeds for the whole state. Checked live on 2026-11-17.

## Access

- Developer page: `https://www.cttransit.com/about/developers`
- Auth: none. No key or sign-up.
- Terms: `https://www.cttransit.com/about/developers/terms-of-use`. Using the feeds accepts them. Free, revocable, as-is, no CTtransit trademarks, and they may charge later.
- No documented rate limit and no rate-limit headers. Poll realtime every 15 to 30 seconds.
- Listed in the MobilityData catalog as `mdb-553` (static) and `mdb-2422`, `mdb-2423`, `mdb-2424` (realtime).

## Static schedule

- URL: `https://www.cttransit.com/sites/default/files/gtfs/googlect_transit.zip` (about 12 MB)
- Mirror: `https://files.mobilitydatabase.org/mdb-553/latest.zip`
- One feed covers Hartford, New Haven, Stamford, New Britain, Waterbury-Meriden, and CTfastrak. New Haven is `agency_id=2`, 29 routes: 201 to 282, 567, and USS.
- The schedule on 2026-11-17 runs 2026-08-30 to 2027-01-09. Check `calendar.txt` daily and fetch the new zip when the current one is about to end. `Last-Modified` on the zip shows a new release.

Parsing:

- There is no `feed_info.txt`.
- `trips.txt` and some other files start with a UTF-8 byte order mark. Strip it or the first column name is wrong.
- `route_id` is a number like `16470`. Riders know the route by `route_short_name`.
- Dead URLs: `googlectransit.zip` without the underscore, and the per-city zips such as `googlenh_transit.zip`.

## Realtime

Base: `https://cttprdtmgtfs.ctttrpcloud.com/TMGTFSRealTimeWebService`

| Feed | Path | Size |
| --- | --- | --- |
| Vehicle positions | `/Vehicle/VehiclePositions.pb` | 27 KB, 332 vehicles statewide, 75 in New Haven |
| Trip updates | `/TripUpdate/TripUpdates.pb` | 488 KB |
| Alerts | `/Alert/Alerts.pb` | empty on 2026-11-17 |

- GTFS-realtime 2.0, `FULL_DATASET`. The header timestamp was about 23 seconds old.
- Each path also has a `.json` twin. Use `.pb`: trip updates are 3.8 MB as JSON.
- Statewide. Keep only entities whose `route_id` belongs to `agency_id=2`.
- IDs join to the static feed: 327 of 332 vehicle `trip_id`s and 8794 of 8807 trip-update `stop_id`s matched.
- Dead URLs: `s3.amazonaws.com/cttransit-realtime-prod/*` returns `403`, and `65.213.12.244/realtimefeed/*` is retired.

## Building a bus leg

1. Board stops: stops within walking range of the origin. Alight stops: stops within walking range of the destination.
2. Find trips in today's service that visit a board stop and later an alight stop.
3. Scheduled times come from `stop_times.txt`. Shift them by the trip's delay from trip updates when one exists.
4. Drop a trip the student cannot walk to in time.
5. Geometry: slice the trip's shape in `shapes.txt` between the two stops using `shape_dist_traveled`.
6. Live position: the trip's vehicle from vehicle positions.

Until we log enough to measure it, p90 is p50 plus 3 minutes for a trip with realtime data and plus 6 for one without.

Shore Line East and the Hartford Line publish separate feeds. They are not modes in `docs/product.md`.
