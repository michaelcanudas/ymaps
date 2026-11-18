# Frontend

Walter owns this directory. Start with [../AGENTS.md](../AGENTS.md), [../docs/product.md](../docs/product.md), [../docs/frontend.md](../docs/frontend.md), and [../contract/openapi.yaml](../contract/openapi.yaml).

## Stack

- Expo (React Native + react-native-web), TypeScript. The web app comes first. iOS and Android come from the same code later.
- The map is MapLibre GL with OpenFreeMap vector tiles (no API key) and a hand-written navy style in `src/map/navyStyle.ts`.
- `src/map/MapCanvas.web.tsx` is the web map. `src/map/MapCanvas.tsx` is the native placeholder until `@maplibre/maplibre-react-native` is added in the mobile phase.

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
- `GET /v1/routes/{routeId}` re-scores the route every 30 seconds while the student moves. A `404` triggers a fresh plan.
- When location is unavailable, a tap on the map sets the start position.
- The map draws only what the route carries: each leg's path, the whole shuttle or bus line faintly behind its leg, that leg's live vehicle, and the bike docks and Veo drop-off spot.

## Layout

```
App.tsx                 screen composition
src/api/                contract types, client, polyline decoder
src/map/                MapLibre canvas, navy style, marker images
src/trip/               top bar, route card, leg strip, timing labels
src/state/              trip and location
src/theme/              palette and mode colors and glyphs
```
