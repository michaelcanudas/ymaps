# Frontend

Walter owns `frontend/` and every surface around it: the mobile app, Siri, the widget, and the top black bar. Michael's agents leave that work alone.

## Stack

Expo (React Native + web) with MapLibre and OpenFreeMap tiles. The web app ships first, and the Apple surfaces come later. Details are in `frontend/README.md`.

## Rules

- Read `contract/openapi.yaml` before adding a screen that talks to the server.
- Call only operations in that file. Generate the client from it, or write the calls by hand against it. Hand-written calls still match the schemas.
- Send the destination. Do not rank modes, predict the shuttle, or resolve aliases on the device.
- Hold to talk captures where the student wants to go and sends that utterance once the contract has an operation for it.
- The top black bar stays on screen and can start that destination action without a long navigation.
- Until an operation exists, build the surface against the current contract. Do not invent a stand-in backend.

## Current server

Deployed: `https://backend-production-95061b6f.up.railway.app`. Local: `http://localhost:8080`.

The client calls `POST /v1/routes` and `GET /v1/routes/{routeId}`. See `contract/openapi.yaml`.
