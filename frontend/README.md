# Frontend

Walter owns this directory. Start with [../AGENTS.md](../AGENTS.md), [../docs/product.md](../docs/product.md), [../docs/frontend.md](../docs/frontend.md), and [../contract/openapi.yaml](../contract/openapi.yaml).

The server is `GET http://localhost:8080/v1/hello`.

## Frontends

- [ios/](ios/README.md) — native SwiftUI app, home-screen widget, and the top bar Live Activity
- The web app in this directory, described below

Both share one look: the navy palette, one pastel color per mode (walk `#8CC8FF`, bike `#5ED39B`, Veo `#F59AC0`, shuttle `#AFA3FF`, bus `#F2B66D`), and the white 3D vehicle sprites rendered in `../../icons/build`.

## Web stack

- Expo (React Native + react-native-web), TypeScript.
- The map is MapLibre GL with OpenFreeMap vector tiles (no API key) and a hand-written navy style in `src/map/navyStyle.ts`.
- `src/map/MapCanvas.web.tsx` is the web map. `src/map/MapCanvas.tsx` is the native placeholder.
- `public/vehicles/` holds the map sprites (buses at every 10° of heading). `assets/modes/` holds the mode icons for chips. Both come from `python ../../icons/build/export_to_app.py`.

## Run

```sh
cd frontend
npm install
echo EXPO_PUBLIC_YMAPS_URL=https://backend-production-95061b6f.up.railway.app > .env.local
npm run web
```

The app opens at `http://localhost:8081`. Without `.env.local`, it calls `http://localhost:8080`.

## How it uses the contract

- `POST /v1/routes` sends the student's position and the destination as typed.
- A `422` with places shows "Which one?". Picking a place retries the request with its `placeId`.
- The route card shows the chosen route and its `alternatives`. "Recommended" and each labeled alternative can be picked.
- Start makes the trip live: `GET /v1/routes/{routeId}` re-scores the picked route every minute until End. A `404` triggers a fresh plan.
- The mode toggles send `preferences.avoidModes` for any mode switched off, and nothing when every mode is on. The choice is saved in `localStorage`.
- When location is unavailable, a tap on the map sets the start position.
- The map draws only what the route carries: each leg's path, the whole shuttle or bus line faintly behind its leg, transfer points, that leg's live vehicle facing its heading, the bike dock and the Veo vehicle.

## Layout

```
App.tsx                 screen composition
src/api/                contract types, client, polyline decoder
src/map/                MapLibre canvas, navy style, marker images
src/trip/               top bar, mode picker, route card, leg chips, timing labels
src/state/              trip, preferences, and location
src/theme/              palette and mode colors and glyphs
```
