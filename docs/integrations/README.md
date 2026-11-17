# Integrations

Data sources the backend reads to build routes. These are not product operations. Nothing here is callable by a client until it is expressed in `contract/openapi.yaml`.

| Mode | Provider | Format | Auth | Vehicles | Drop-off spots |
| --- | --- | --- | --- | --- | --- |
| Ride New Haven bikes | Drop Mobility | GBFS 2.x | Bearer key | Yes | Yes, 65 virtual stations |
| Veo scooters and e-bikes | Veo | GBFS 2.2 | None | Yes | Not in the public feed |
| Yale Shuttle | Downtowner | JSON | None | Yes | Stops |
| New Haven bus | CTtransit | GTFS and GTFS-realtime | None | Yes | Stops |

Places come from a separate source:

| Data | Provider | Format | Auth |
| --- | --- | --- | --- |
| Yale buildings, abbreviations, entrances | Concept3D (map.yale.edu) | JSON | Public client key |

- [Ride New Haven](ride-new-haven.md)
- [Veo](veo.md)
- [Yale Shuttle](yale-shuttle.md)
- [CTtransit](cttransit.md)
- [Yale map](yale-map.md)

How these feed the router: [routing.md](../routing.md).

Both feeds follow [GBFS](https://github.com/MobilityData/gbfs). One GBFS reader in the backend can serve both, with per-provider config for base URL, auth, and which feeds exist.

## Secrets

Keys live in `.env` at the repo root, which is gitignored. Never commit a key or paste it into docs.

| Variable | Provider |
| --- | --- |
| `RIDE_NEW_HAVEN_GBFS_KEY` | Drop Mobility |
| `YALE_MAP_KEY` | Concept3D |
