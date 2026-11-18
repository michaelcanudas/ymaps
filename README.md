# ymaps

**The best way across Yale, in the time it takes to say where you're going.**

![ymaps](<Screenshot 2026-11-17 190945.png>)

## The problem

Getting around Yale means juggling five ways to move: walking, Veo scooters, Ride New Haven bikes, the Yale Shuttle, and the CTtransit bus. Each has its own app, its own price, and its own reliability. And no map knows Yale. Google doesn't know what "WLH" means, where ACCT 270 meets, or which entryway gets you into your college.

## The idea

Say or type where you're going. ymaps gives you the single best route, right away.

## Our inspiration

When Steve Jobs built the iPod, he held it to two constraints: 1000 songs in your pocket, and any song in under four seconds. Anything that didn't serve those, like FM radio, was left out.

ymaps follows the same rule with two constraints of its own:

1. **The optimal route.** The best way there, across every mode, with live conditions and campus knowledge a map doesn't have.
2. **The fastest time to get it.** As little friction as possible between you and that route.

A feature that doesn't serve one of them doesn't ship.

## Why it's cool

- **Every mode, one answer.** Bike → bus, shuttle → shuttle, or walking the whole way all come out of a single search over live data. You don't compare five apps yourself.
- **It speaks Yale.** `WLH`, `silliman buttery`, and `ACCT 270` take you to the right building.
- **Live and self-correcting.** Shuttle, bike, scooter, and bus feeds update continuously. If a leg gets worse mid-trip, you get a better route.
- **One answer, not ten.** You get the best route plus up to two alternatives, each with a short trade-off label like "free" or "less walking."
- **Powered by Grok.** Grok turns what you actually say ("the place by Commons with the good bagels") into a destination.

## See it work

- [Web app demo](media/web-demo.mp4)
- [iOS app demo](media/ios-demo.mp4)

<video src="media/web-demo.mp4" controls muted width="720"></video>

<video src="media/ios-demo.mp4" controls muted width="320"></video>

---

Building ymaps? Start with [AGENTS.md](AGENTS.md) and [docs/](docs/).
