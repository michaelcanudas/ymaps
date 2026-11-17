# Frontend

Walter owns `frontend/` and every surface around it: the mobile app, Siri, the widget, and the top black bar. Michael's agents leave that work alone.

## Stack

Walter chooses the stack. The working assumption is React Native for the app, with the Apple surfaces beside it. Document the choice in `frontend/README.md` when the app exists.

## Rules

- Read `contract/openapi.yaml` before adding a screen that talks to the server.
- Call only operations in that file. Generate the client from it, or write the calls by hand against it. Hand-written calls still match the schemas.
- Send the destination. Do not rank modes, predict the shuttle, or resolve aliases on the device.
- Hold to talk captures where the student wants to go and sends that utterance once the contract has an operation for it.
- The top black bar stays on screen and can start that destination action without a long navigation.
- Until an operation exists, build the surface against the current contract. Do not invent a stand-in backend.

## Current server

`GET http://localhost:8080/v1/hello` → `{"message":"hello"}`.
