# iOS

Native SwiftUI app. No dependencies, no code generators.

```
App/       the app: a dark map, one "Where to?" field, the route card, Start/End
Widget/    the widget extension: the "Where to?" home-screen widget and the top bar Live Activity
Shared/    code compiled into both targets
Config/    signing settings, the app Info.plist additions, and the widget Info.plist
```

The folders are synchronized groups, so a new Swift file dropped into a folder joins its target without touching the project file.

## Signing

`Config/Signing.xcconfig` is checked in with no team. Create `Config/Signing.local.xcconfig`, which git ignores:

```
DEVELOPMENT_TEAM = <your team id>
BUNDLE_ID_PREFIX = com.<you>
```

A free personal team works. The bundle ids become `<prefix>.ymaps` and `<prefix>.ymaps.widget`.

## Run on a phone

1. Xcode → Settings → Accounts: sign in with your Apple ID.
2. On the phone: Settings → Privacy & Security → Developer Mode → on, then restart.
3. Plug in the phone, trust the Mac.
4. Open `ymaps.xcodeproj`, choose the phone as the destination, press Run.
5. First launch only, on the phone: Settings → General → VPN & Device Management → trust your developer certificate.

From the command line:

```sh
xcodebuild -project ymaps.xcodeproj -scheme ymaps -destination 'id=<device udid>' -derivedDataPath build -allowProvisioningUpdates -allowProvisioningDeviceRegistration build
xcrun devicectl device install app --device <device id> build/Build/Products/Debug-iphoneos/ymaps.app
```

`xcrun devicectl list devices` prints the device id. `xcrun devicectl device info details --device <device id>` prints the udid. Build against the udid, not `generic/platform=iOS`, so the phone is registered in the provisioning profile.

## Server

The app calls `YmapsServerURL` from `Config/AppInfo.plist`, the Railway backend. To point a simulator at a local backend, launch with an override:

```sh
xcrun simctl launch <simulator udid> <bundle id> -server http://localhost:8090
```

## Try the surfaces

- App: type a destination and press Return. An ambiguous one lists places to pick. Start shows the top bar; End removes it.
- Top bar: after Start, the Dynamic Island and lock screen show the current step and a countdown to arrival. The route refreshes every minute.
- Widget: long-press the home screen → Edit → Add Widget → ymaps → Where to. Tapping it opens the app ready to type.
