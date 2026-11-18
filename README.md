# ymaps

ymaps gets a Yale student the optimal route in the fastest time.

Getting around Yale means choosing between walking, Veo scooters, Ride New Haven bikes, the Yale Shuttle, and the CTtransit bus, each with its own cost, reliability, and app. Google Maps doesn't know what "WLH" means, where ACCT 270 meets, or which entryway gets you into your college. ymaps does. Say or type where you're going and you get the single best route right away.

**Inspiration.** Steve Jobs held the iPod to two constraints: 1000 songs in your pocket, and any song in under four seconds. Anything that didn't serve those, like FM radio, was left out. ymaps keeps the same discipline with two constraints of its own: **the optimal route** and **the fastest time to get it**. A feature that doesn't serve one of them doesn't ship.

**What makes it cool**

- **One search across every mode.** Shuttle → bus, bike → shuttle, or walking the whole way all come out of a single planner running on live data, not separate apps you compare yourself.
- **It speaks Yale.** `WLH`, `silliman buttery`, and `ACCT 270` resolve to the right building, using the Yale map, a course-to-building table, and our own aliases.
- **Live and self-correcting.** It polls shuttle, bike, scooter, and bus feeds continuously, and the route is re-scored while you travel. If a leg gets worse, you get a better route.
- **Real streets.** Walk and ride times come from a self-hosted Valhalla on OpenStreetMap, not straight lines.
- **One answer, not ten.** You get the best route plus up to two alternatives, each with a short trade-off label.

Two people build it in this repo. Michael owns `backend/`. Walter owns `frontend/`. They meet only through `contract/openapi.yaml`.

![Brand and visual direction](<Screenshot 2026-11-17 190945.png>)

## Demo

- [Web app](media/web-demo.mp4)
- [iOS app](media/ios-demo.mp4)

<video src="media/web-demo.mp4" controls muted width="720"></video>

<video src="media/ios-demo.mp4" controls muted width="320"></video>

## Start here

1. [AGENTS.md](AGENTS.md) if you are an agent
2. [docs/product.md](docs/product.md) for what the product is and what it refuses to be
3. [docs/architecture.md](docs/architecture.md) for the split and the contract
4. [docs/workflow.md](docs/workflow.md) for commits
5. [docs/backend.md](docs/backend.md) or [docs/frontend.md](docs/frontend.md) for your side

## Run the backend

```sh
cd backend
go run ./cmd/ymaps
```

`GET http://localhost:8080/v1/hello` returns `{"message":"hello"}`. Override the port with `PORT`.
