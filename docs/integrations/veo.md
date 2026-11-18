# Veo

E-scooters and e-bikes operated by Veo under a City of New Haven program launched in May 2025. Riders must end a trip at a designated station, so drop-off spots are required for a correct Veo leg.

## Access

- Entry point: `https://cluster-prod.veoride.com/api/shares/name/nhv/gbfs`
- Public. No key, no sign-up.
- Listed in the MobilityData GBFS catalog as `veo-nhv` ([systems.csv](https://github.com/MobilityData/gbfs/blob/master/systems.csv)).
- GBFS 2.2. Paths have no `.json` suffix.
- `ttl` is `0`. Poll on our own interval.

## Feeds

Base: `https://cluster-prod.veoride.com/api/shares/name/nhv/gbfs`

| Feed | Path | Use |
| --- | --- | --- |
| System information | `/system_information` | Name, app links |
| Free bike status | `/free_bike_status` | Every vehicle and its location |
| Vehicle types | `/vehicle_types` | Scooter or bike, range |
| System pricing plans | `/system_pricing_plans` | Cost of a leg |

`station_information`, `station_status`, and `geofencing_zones` return `404`. No Veo city in the catalog publishes them.

`system_information.timezone` says `America/Chicago`, which is wrong for New Haven. Use the epoch timestamps and ignore that field.

## Vehicles

`free_bike_status` → `data.bikes[]`

```json
{
  "bike_id": "2da170b0-cbf3-5fd1-91e1-09d4fd52d8d8",
  "lat": 41.30301,
  "lon": -72.918091,
  "is_reserved": false,
  "is_disabled": false,
  "vehicle_type_id": "1",
  "pricing_plan_id": "54",
  "current_range_meters": 12261,
  "rental_uris": { "ios": "https://gmjc.adj.st/?adj_t=5vyf0nr&number=2004624", "android": "…" }
}
```

- Skip `is_disabled` or `is_reserved`.
- `current_range_meters` lets us drop vehicles that cannot finish the leg.
- `rental_uris` deep-links into the Veo app for that vehicle. Useful for fastest time if the contract ever carries it.

## Vehicle types

| `vehicle_type_id` | Form | Propulsion | Max range |
| --- | --- | --- | --- |
| `0` | bicycle | human | — |
| `1` | scooter | electric | 52.8 km |
| `2` | bicycle | electric assist | 45 km |
| `3` | bicycle | electric | 67 km |
| `4` | scooter | electric | 67 km |
| `5` | scooter | electric | 67 km |

## Pricing

Every plan currently reads $1.00 unlock plus $0.40 per minute (`price` plus `per_min_pricing`). Read it from the feed rather than hard-coding it.

## Snapshot, 2026-11-17

319 vehicles: 293 type `1` scooters, 26 type `3` e-bikes. 3 disabled, 5 reserved.

## Building a Veo leg

- Start: a vehicle that is not disabled or reserved, with `current_range_meters` at least 1.3 × the ride distance plus 500 m.
- End: a drop-off spot near the destination. See below.
- Cost: from the vehicle's `pricing_plan_id`. Plans are marked `is_taxable`; Connecticut sales tax is 6.35%.
- Speed: scooter 13 km/h, e-bike 15 km/h. Add 3 minutes to find, unlock, and park, since positions jitter 5 to 50 m and parking needs a photo.
- Still there on arrival: discount a vehicle by how long the walk is. Fall back to other vehicles at the same spot.

## Drop-off spots: open

The city launched the program with about 50 stations in Downtown, Dwight, Dixwell, East Rock, Wooster Square, and the Hill. Station locations are shown only inside the Veo app. No public dataset or endpoint was found.

Ways to get them, in order:

1. **Ask Doug Hausladen.** The city runs the permit and likely receives station or MDS data from Veo. He already offered help on the Ride New Haven thread.
2. **Ask Veo directly** (`hello@veoride.com`, from `system_information`) for a station list or a `station_information` feed for `nhv`, framed the same way as the Drop request: availability only, no booking.
3. **Derive them.** This works better than expected; see below.

Do not scrape the Veo app's private API.

## Deriving drop-off spots

Parked vehicles cluster tightly. In one snapshot on 2026-11-17 the median distance from a vehicle to its nearest neighbor was 2 m. Grouping vehicles within 30 m of each other, at least 3 to a group, gave 35 groups holding 230 of 316 vehicles, most 6 to 40 m across. That fits the roughly 50 announced stations. One group of 45 at 41.3158, -72.9230 looks like a depot, not a station.

`bike_id` stayed the same across four snapshots, including for 3 vehicles that moved more than 50 m. If that holds over a day, a trip's end point is visible directly: a vehicle that moves and then stops. That is a stronger signal than where vehicles happen to rest.

Plan:

1. Log `free_bike_status` for several days.
2. Record trip end points from vehicles whose position changes.
3. Keep spots that persist across days and receive trip ends. Drop depot-sized outliers.
4. Rebuild the spot list daily so moved stations follow.

Confirm over a full day that `bike_id` does not rotate after a trip. GBFS recommends rotating it.

## Learned spots

`internal/veospots` learns drop-off spots from the live feed and hands them to `gbfs.NewVeo` as its `SpotSource`. Learned spots replace the single-snapshot clusters once any qualify.

Method:

1. Every 30 s, read the Veo reader's latest snapshot. Key each vehicle by the `number` in `rental_uris.ios`, since `bike_id` may rotate.
2. Keep each vehicle's resting position. A trip end is a vehicle that is next seen more than 150 m from that position and then stays within 30 m for 2 polls. It does not count if it was away more than 3 hours, or if its `current_range_meters` rose by more than 1 km, since both point to staff charging or rebalancing rather than a ride.
3. Group trip ends within 30 m into spots, with a running centroid, a count, and a last-seen time. A spot with 3 or more trip ends qualifies.
4. Until 10 spots qualify, use resting vehicles instead: a vehicle that stays within 30 m for 10 minutes adds one rest to a 30 m group, and a group with 3 or more rests qualifies.
5. Drop any group that ever held more than 30 vehicles at once (the depot), groups not seen for 30 days, and groups that never qualified within 7 days. Tracks, groups, and the recent trip end list are capped.

State is saved as JSON every 5 minutes and on shutdown, to `VEO_SPOTS_PATH` (default `./data/veo_spots.json`), and loaded on start. On Railway, point it at a mounted volume.

Debug views, served only when `YMAPS_DEBUG=1`. They are not product operations and are not in the contract.

- `GET /debug/veo/spots`: the active source (`tripEnds`, `rests`, or `none`), every group with `lat`, `lon`, `tripEnds` or `rests`, `peakVehicles`, `qualified`, `lastSeen`, and the most recent trip ends.
- `GET /debug/veo/spots.geojson`: the same as a `FeatureCollection` of points, with `kind` set to `tripEndSpot`, `restSpot`, or `tripEnd`.

A public dataset of station locations was looked for again on 2026-11-17 and none was found.

## Live behavior

Seen on 2026-11-17:

- `last_updated` equals the request time, so the feed is built on demand and always looks fresh. Detect an outage from failed requests, or from no vehicle moving for several minutes.
- No vehicle has its own `last_reported`.
- 3 vehicles had `current_range_meters: 0` and 9 had under 2 km, some not marked disabled. Filter by range.
- Coordinates are mostly 5 to 6 decimals, a few 14 to 15. Round before comparing.
- Four pricing plans exist (54, 303, 382, 509). Only 54 (scooters) and 303 (e-bikes) are in use, at the same price.
- 318 vehicles.
