# Ride New Haven

Bike share operated by Drop Mobility. Access was granted on 2026-11-16 by Amber Wason (VP Growth & Innovation, Drop Mobility) after an introduction from Doug Hausladen (New Haven Parking Authority). The key was issued for "Yale School Project". Drop asked to see the tool once it is available.

Terms of the request: we show availability and use it in routing. Students do not book through ymaps.

## Access

- Entry point: `https://gbfs.dropmobility.com/systems/newhaven/gbfs.json`
- Auth: header `Authorization: Bearer $RIDE_NEW_HAVEN_GBFS_KEY`. An `authkey` query parameter also works; prefer the header so the key stays out of logs.
- Without a key the feeds return `400`.
- `access-control-allow-origin: *`, but the key must stay on the server. Clients get bike data only through the contract.
- `ttl` is `0` on every feed. Pick a poll interval ourselves; 30 to 60 seconds is typical for GBFS.

## Feeds

Base: `https://gbfs.dropmobility.com/systems/newhaven`

| Feed | Path | Use |
| --- | --- | --- |
| System information | `/system_information.json` | Name, timezone `America/New_York` |
| Station information | `/station_information.json` | Drop-off spots |
| Station status | `/station_status.json` | Bikes available per spot |
| Vehicle types | `/vehicle_types.json` | Bike kinds and range |
| Free bike status | `/free_bike_status.json` | Bikes parked away from a station |

The `version` field reads `2.0` on some feeds and `3.0` on others, but every payload has the GBFS 2.x shape (`free_bike_status`, `bike_id`). Parse as 2.x.

## Drop-off spots

`station_information.json` → `data.stations[]`

```json
{
  "station_id": "415e9790-83f6-4553-8cb6-c90e8c27f6aa",
  "name": "Elm and Orange",
  "lat": 41.307273135851034,
  "lon": -72.92240905444065,
  "is_virtual_station": 1
}
```

All 65 stations are virtual: painted or signed parking areas with no docks. There is no capacity or `num_docks_available`, so a spot never "fills up". Some names have trailing spaces (`"Yale Post Office "`); trim them.

`station_status.json` → `data.stations[]`

```json
{
  "station_id": "8e1750e8-a2c3-4f18-97d1-2efdf7907faf",
  "num_bikes_available": 3,
  "is_installed": true,
  "is_renting": true,
  "is_returning": true,
  "last_reported": 1794955861,
  "vehicles": [
    { "bike_id": "fee3b93e-…", "is_reserved": false, "is_disabled": false, "vehicle_type_id": "ebike" }
  ]
}
```

- `is_returning` decides whether a spot is a valid end of a bike leg.
- `is_renting` and `num_bikes_available` decide whether a spot is a valid start.
- `vehicles` is `null`, not `[]`, when a spot is empty.

## Bikes

Bikes at stations come from `station_status.vehicles`. Bikes left elsewhere come from `free_bike_status.json` → `data.bikes[]`:

```json
{
  "bike_id": "newhaven_0",
  "lat": 41.29814,
  "lon": -72.92715333333334,
  "is_disabled": false,
  "is_reserved": false,
  "vehicle_type_id": "ebike"
}
```

Free-floating `bike_id` values look like `newhaven_N` and are not the same ids used under stations. Do not join them. Skip bikes where `is_disabled` or `is_reserved` is true.

No battery or range per bike is published.

## Vehicle types

| `vehicle_type_id` | Name | Propulsion | Max range |
| --- | --- | --- | --- |
| `ebike` | Cargo Bike | electric assist | 40 km |
| `pedal_bike` | Pedal Bike | human | — |

## Snapshot, 2026-11-17

65 stations, 128 bikes at stations, 3 free-floating bikes, all `ebike`.

## Not in the feed

- Pricing (`system_pricing_plans` is not published). It lives in backend config instead; see below.
- Service area or no-ride zones (`geofencing_zones` is not published)

## Pricing

From [ridenewhaven.com](https://ridenewhaven.com), checked 2026-11-17:

| Plan | Price |
| --- | --- |
| E-bike, pay per ride | $1.00 unlock + $0.25 per minute |
| Pedal bike, pay per ride | $1.00 unlock + $0.10 per minute |
| Monthly member | $20 per month, 60 free minutes a day, then $0.15 per minute |

There is no student price. Recheck these when the site changes.

## Building a bike leg

- Start: a station with `is_renting` and `num_bikes_available > 0`, or a free-floating bike.
- End: a station with `is_returning`, among the nearest few to the destination.
- Range: ignore. 40 km is far beyond any trip in town, and no per-bike charge is published.
- Speed: the e-bike is a heavy cargo bike. Use 13 km/h, plus 2 minutes to unlock and dock.
- Still there on arrival: estimate how fast each station loses bikes from successive `station_status` polls, and discount a station whose last bike may be gone by the time the student walks there.

## Live behavior

Seen on 2026-11-17:

- `gbfs.json` `last_updated` is from 2023 and `system_information` from 2020. Judge freshness only from `station_status` and `free_bike_status`.
- `last_reported` is the same on all 65 stations. It is when the feed was built, not a per-station heartbeat.
- `num_bikes_available` matched the length of `vehicles` on every station.
- 13 station names had trailing spaces. 21 stations had `vehicles: null`.
- 4 free-floating bikes, all `ebike`.

## Related

Doug also pointed at [Via New Haven](https://city.ridewithvia.com/nhv), the city's microtransit pilot, which hands riders off to CTtransit when a bus line is nearby. Not a mode in `docs/product.md` yet.
