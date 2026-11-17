# Product

Steve Jobs told Elon Musk that focus is the whole design philosophy: decide what makes the product great, and ignore everything else. The iPod was built against two constraints, 1000 songs and search in under four seconds. FM radio never got in, because it did not serve either constraint.

ymaps uses the same rule. The product is transportation for Yale students. There are too many ways to move, and they trade off against each other: Veo scooters, Ride New Haven bikes, walking, the Yale Shuttle, and the New Haven bus. Cost and unpredictability differ. Local knowledge is missing: which entryway, what "WLH" means, where ACCT 270 meets.

## The two constraints

1. **Optimal route.** The best way to go, given the modes above, their cost, their reliability, live conditions, and campus facts a map does not know.
2. **Fastest time.** The student gets that route with as little friction as possible.

A change ships only if it improves the route or shortens the time to receive it. Anything else is FM radio.

## Optimal route

Route quality comes from the backend. The work includes:

- Live data from the mode providers
- Shuttle arrival prediction that improves as new observations arrive
- Rerouting when a leg gets worse
- A lexicon from student language to places (`WLH` is William L. Harkness Hall, `ACCT 270` is the building where that course meets)
- Crowdsourced facts maps lack, starting with entryways

## Fastest time

Speed comes from the surfaces Walter owns:

- A mobile app
- Siri
- A home-screen widget
- A top black bar that stays available for the destination
- Hold to talk: the student says where they are going

The spoken or typed destination is sent to the backend. The backend resolves it and returns the route. The client does not choose the route.

## Words both sides use

- **Destination** — what the student said or typed
- **Place** — a resolved campus location
- **Alias** — a term that maps to a place or a course
- **Entryway** — a specific way into a building or college
- **Mode** — one way of traveling
- **Leg** — one mode segment of a trip
- **Route** — the ordered legs that get the student there

Use these words in the contract, the code, and the interface. Do not invent synonyms.

## Grok

Any model, speech, ranking, or generated language in the product uses a Grok product. Transit, map, and mobility HTTP APIs are data sources, not model providers.

## Not in the contract yet

The items under "Optimal route" are the product. They are not endpoints until they appear in `contract/openapi.yaml`. Do not call them, stub them, or shape a client around a guess.
