# Yale map

The source of Yale places: buildings, their abbreviations, their entrances, and the dining halls. `https://map.yale.edu/?id=1910` runs on Concept3D (Atlas). Its JSON API is public. Checked live on 2026-11-17.

## Access

- Base: `https://api.concept3d.com`
- Map: `1910`
- Key: a client key that ships in the site's JavaScript (`https://map.yale.edu/js/index.js`, the `key=` string). Store it as `YALE_MAP_KEY` in `.env` like any other key. If the site changes its key, read the new one from the same file.
- No login. Admin and preview use a separate auth we do not need.

## Endpoints

| Endpoint | Returns |
| --- | --- |
| `GET /locations?map=1910&key=…` | All 977 locations, 1.2 MB |
| `GET /locations/{id}?map=1910&key=…` | One location with full detail |
| `GET /categories?map=1910&key=…` | 40 categories, nested by `parent` |
| `GET /search?map=1910&key=…&q=WLH` | `{resultsPerPage, totalFound, data}` |

`cat=` on `/locations` is ignored. Filter by `catId` ourselves.

## Locations

List records carry `id`, `catId`, `name`, `lat`, `lng`, `locationType` (`standard`, `parking`, `access_point`, `space`), and `shape`, a building outline as `[lat, lng]` pairs.

Detail records add:

- `description`: HTML with `Address:`, `Abbreviation:`, accessibility notes, and departments in the building
- `keywords`: comma-separated, such as `AKW,Arthur K. Watson,Computer Science`
- `preferredEntrances`, `categoryName`, `mediaUrls`

Building abbreviations have no field of their own. Parse them from `Abbreviation:` in `description` and from `keywords`. That needs one detail call per building, about 354, which only has to happen when the location list changes.

Example: Watson Hall, id `560119`, 41.313107, -72.92485, abbreviation `AKW`. William L. Harkness Hall (`WLH`) is id `560127`.

## Categories that matter

| Category | Count | Use |
| --- | --- | --- |
| Buildings | 354 | Places and abbreviations |
| Accessible entrances | 218 | Entryways |
| Inaccessible entrances | 88 | Entryways |
| Residential Dining | 14 | One per college |
| Retail Dining | 8 | |
| Undergraduate Housing | 25 | |
| Libraries | 12 | |

Entrances are separate points named like `…, Whitney Avenue entrance`. Nothing links an entrance to its building. Match by the name prefix, then by the nearest building outline.

There are no categories for butteries, entryways inside a college, or rooms. Those come from our own aliases.

## Course locations

[ferry](https://github.com/coursetable/ferry) and [coursetable](https://github.com/coursetable/coursetable) store no coordinates. Two pieces of theirs are useful:

- `ferry/crawler/classes/parse.py`, `extract_meetings`: Yale Course Search lists each meeting place with a link such as `https://map.yale.edu/?id=1910#!m/560119`. The number after `m/` is the Concept3D location id.
- `ferry/transform/import_courses.py`, `parse_location`: turns `WLH 120`, `WLH`, `WLH - Name`, and `WLH 120 - Name 120` into building code, room, and name.

`coursetable/frontend/src/generated/buildings.json` lists 154 building codes, 97 with a map link. Together with the parsed abbreviations, this maps a course's meeting place to a Concept3D location and its coordinates.
