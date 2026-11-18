import MapKit
import SwiftUI

struct RootView: View {
    @Environment(Trip.self) private var trip
    @State private var camera: MapCameraPosition = .userLocation(fallback: .region(RootView.campus))
    @Binding var searchRequest: Int
    #if LIVE_SHUTTLES
    @State private var shuttles = LiveShuttles()
    #endif

    static let world = [
        CLLocationCoordinate2D(latitude: 85, longitude: -179.9),
        CLLocationCoordinate2D(latitude: 85, longitude: 179.9),
        CLLocationCoordinate2D(latitude: -85, longitude: 179.9),
        CLLocationCoordinate2D(latitude: -85, longitude: -179.9),
    ]

    static let campus = MKCoordinateRegion(
        center: CLLocationCoordinate2D(latitude: 41.3105, longitude: -72.9260),
        span: MKCoordinateSpan(latitudeDelta: 0.018, longitudeDelta: 0.018)
    )

    var body: some View {
        map
            .ignoresSafeArea()
            .safeAreaInset(edge: .bottom) {
                TripPanel(searchRequest: searchRequest)
                    .dynamicTypeSize(...DynamicTypeSize.xLarge)
                    .padding(.horizontal, 12)
                    .padding(.bottom, 4)
            }
            .background(Brand.mapBackground)
            #if LIVE_SHUTTLES
            .task { await shuttles.run() }
            #endif
            .onChange(of: trip.route) { _, route in
                guard let route else { return }
                withAnimation(.easeInOut(duration: 0.5)) { camera = RootView.framing(route) }
            }
    }

    private var map: some View {
        Map(position: $camera) {
            MapPolygon(coordinates: RootView.world)
                .foregroundStyle(Brand.mapBackground.opacity(0.5))
                .mapOverlayLevel(level: .aboveRoads)
            UserAnnotation()
            #if LIVE_SHUTTLES
            ForEach(shuttles.buses) { bus in
                Annotation("", coordinate: bus.coordinate.location) {
                    Image(systemName: "location.north.fill")
                        .font(.system(size: 9, weight: .bold))
                        .foregroundStyle(.black)
                        .rotationEffect(.degrees(bus.heading))
                        .frame(width: 18, height: 18)
                        .background(Color(hex: bus.color), in: Circle())
                        .animation(.linear(duration: 5), value: bus.coordinate)
                }
            }
            #endif
            if let route = trip.route {
                ForEach(Array(route.legs.enumerated()), id: \.offset) { _, leg in
                    if let line = leg.line, Polyline.decode(line.path).count > 1 {
                        MapPolyline(coordinates: Polyline.decode(line.path))
                            .stroke(Color(hex: line.color).opacity(0.25), lineWidth: 3)
                    }
                    if leg.isRide, Polyline.decode(leg.path).count > 1 {
                        MapPolyline(coordinates: Polyline.decode(leg.path))
                            .stroke(Brand.ink.opacity(0.9), style: StrokeStyle(lineWidth: 9, lineCap: .round, lineJoin: .round))
                    }
                    if Polyline.decode(leg.path).count > 1 {
                        MapPolyline(coordinates: Polyline.decode(leg.path))
                            .stroke(RootView.color(for: leg), style: RootView.stroke(for: leg))
                    }
                }
                ForEach(Array(route.legs.dropFirst().enumerated()), id: \.offset) { _, leg in
                    Annotation("", coordinate: leg.from.coordinate.location) {
                        Circle()
                            .fill(Brand.ink)
                            .stroke(Brand.textPrimary, lineWidth: 2.5)
                            .frame(width: 11, height: 11)
                    }
                }
                ForEach(Array(route.legs.enumerated()), id: \.offset) { _, leg in
                    if let marker = RootView.vehicleMarker(for: leg) {
                        Annotation("", coordinate: marker.coordinate, anchor: .center) {
                            Image(marker.sprite)
                                .resizable()
                                .interpolation(.high)
                                .frame(width: VehicleSprite.size(for: leg.mode), height: VehicleSprite.size(for: leg.mode))
                        }
                    }
                }
                if let last = route.legs.last {
                    Annotation("", coordinate: last.to.coordinate.location) {
                        RoundedRectangle(cornerRadius: 2)
                            .fill(Brand.textPrimary)
                            .frame(width: 16, height: 16)
                            .overlay(Rectangle().fill(Brand.ink).frame(width: 6, height: 6))
                    }
                }
            }
        }
        .mapStyle(.standard(emphasis: .muted, pointsOfInterest: .excludingAll))
        .mapControlVisibility(.hidden)
        .ignoresSafeArea(.keyboard)
    }

    static func color(for leg: Leg) -> Color {
        Brand.color(forMode: leg.mode)
    }

    static func vehicleMarker(for leg: Leg) -> (coordinate: CLLocationCoordinate2D, sprite: String)? {
        switch leg.mode {
        case "shuttle", "bus":
            guard let vehicle = leg.vehicle else { return nil }
            let heading = vehicle.heading.map(Double.init) ?? leg.line.map {
                VehicleSprite.bearing(along: Polyline.decode($0.path), at: vehicle.coordinate.location)
            }
            guard let sprite = VehicleSprite.name(for: leg.mode, heading: heading) else { return nil }
            return (vehicle.coordinate.location, sprite)
        case "bike":
            guard let sprite = VehicleSprite.name(for: leg.mode, heading: nil) else { return nil }
            return (leg.from.coordinate.location, sprite)
        case "veo":
            guard let vehicle = leg.vehicle, let sprite = VehicleSprite.name(for: leg.mode, heading: nil) else { return nil }
            return (vehicle.coordinate.location, sprite)
        default:
            return nil
        }
    }

    static func ink(for leg: Leg) -> Color {
        Brand.ink
    }

    static func stroke(for leg: Leg) -> StrokeStyle {
        leg.mode == "walk"
            ? StrokeStyle(lineWidth: 4, lineCap: .round, dash: [1, 8])
            : StrokeStyle(lineWidth: 6, lineCap: .round, lineJoin: .round)
    }

    static func framing(_ route: Route) -> MapCameraPosition {
        var points = route.legs.flatMap { Polyline.decode($0.path) }
        if points.isEmpty, let last = route.legs.last {
            points = [route.legs[0].from.coordinate.location, last.to.coordinate.location]
        }
        var rect = MKMapRect.null
        for point in points {
            let mapPoint = MKMapPoint(point)
            rect = rect.union(MKMapRect(x: mapPoint.x, y: mapPoint.y, width: 0, height: 0))
        }
        guard !rect.isNull else { return .automatic }
        let side = max(rect.width, rect.height, 800)
        let padded = MKMapRect(
            x: rect.midX - side * 0.7,
            y: rect.minY - side * 0.35,
            width: side * 1.4,
            height: rect.height + side * 1.4
        )
        return .rect(padded)
    }
}

struct TripPanel: View {
    @Environment(Trip.self) private var trip
    let searchRequest: Int
    @State private var query = ""
    @FocusState private var searching: Bool

    var body: some View {
        content
            .foregroundStyle(Brand.textPrimary)
            .animation(.snappy, value: phaseKey)
            .animation(.snappy, value: searching)
            .onChange(of: phaseKey) { _, key in
                if key == "idle" { query = "" }
            }
            .onChange(of: searchRequest) {
                trip.clear()
                searching = true
            }
    }

    @ViewBuilder
    private var content: some View {
        switch trip.phase {
        case .idle:
            searchBar
        case .loading(let destination):
            card {
                HStack {
                    Text(destination).font(.headline).lineLimit(1)
                    Spacer()
                    ProgressView().tint(Brand.textPrimary)
                }
            }
        case .failed(let message):
            VStack(alignment: .leading, spacing: 8) {
                Text(message)
                    .font(.subheadline)
                    .lineLimit(2)
                    .foregroundStyle(Brand.textSecondary)
                    .padding(.horizontal, 8)
                searchBar
            }
        case .choosing(let destination, let places):
            card { choices(destination, places) }
        case .route(let options, let index):
            card { RouteCard(route: options[index], options: options, selected: index) }
        }
    }

    private var phaseKey: String {
        switch trip.phase {
        case .idle: "idle"
        case .loading: "loading"
        case .choosing: "choosing"
        case .failed: "failed"
        case .route(let options, let index): options[index].id
        }
    }

    private func card<Content: View>(@ViewBuilder _ content: () -> Content) -> some View {
        content()
            .padding(18)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(Brand.panel.opacity(0.97), in: RoundedRectangle(cornerRadius: 16, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: 16, style: .continuous).stroke(Brand.hairline, lineWidth: 1))
    }

    private var searchBar: some View {
        HStack(spacing: 10) {
            Image(systemName: "magnifyingglass")
                .foregroundStyle(Brand.textSecondary)
            TextField("", text: $query, prompt: Text("Where to?").foregroundStyle(Brand.textSecondary))
                .font(.body.weight(.medium))
                .focused($searching)
                .submitLabel(.go)
                .autocorrectionDisabled()
                .onSubmit {
                    searching = false
                    trip.go(to: query)
                }
            if searching && !query.isEmpty {
                Button {
                    query = ""
                } label: {
                    Image(systemName: "xmark.circle.fill")
                        .foregroundStyle(Brand.textFaint)
                }
                .buttonStyle(.plain)
            }
            if searching {
                Button("Cancel") {
                    searching = false
                    query = ""
                }
                .font(.subheadline.weight(.medium))
                .foregroundStyle(Brand.textSecondary)
                .buttonStyle(.plain)
                .transition(.opacity)
            }
        }
        .padding(.horizontal, 18)
        .frame(height: 56)
        .background(Brand.panel.opacity(0.97), in: Capsule())
        .overlay(Capsule().stroke(Brand.hairline, lineWidth: 1))
    }

    private func choices(_ destination: String, _ places: [Place]) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Text("Which \(destination)?").font(.headline).lineLimit(1)
                Spacer()
                CloseButton { trip.clear() }
            }
            .padding(.bottom, 6)
            ForEach(places) { place in
                Button {
                    trip.go(to: destination, placeId: place.id)
                } label: {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(place.name).font(.body.weight(.medium)).lineLimit(1)
                        if let address = place.address {
                            Text(address)
                                .font(.caption)
                                .foregroundStyle(Brand.textSecondary)
                                .lineLimit(1)
                        }
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.vertical, 8)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
            }
        }
    }
}

struct RouteCard: View {
    @Environment(Trip.self) private var trip
    let route: Route
    let options: [Route]
    let selected: Int

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack(alignment: .firstTextBaseline) {
                (Text(Duration.long(minutesAway).0).font(.system(size: 40, weight: .bold, design: .rounded))
                    + Text(Duration.long(minutesAway).1).font(.title3.weight(.semibold)))
                    .lineLimit(1)
                    .minimumScaleFactor(0.6)
                Spacer()
                CloseButton { trip.clear() }
            }
            VStack(alignment: .leading, spacing: 2) {
                Text(route.place.entryway?.name ?? route.place.name)
                    .font(.headline)
                    .lineLimit(1)
                Text(subtitle)
                    .font(.subheadline)
                    .lineLimit(1)
                    .minimumScaleFactor(0.85)
                    .foregroundStyle(Brand.textSecondary)
            }
            LegStrip(legs: route.legs)
            if !trip.isLive && options.count > 1 {
                VStack(spacing: 0) {
                    ForEach(options.indices.filter { $0 != selected }, id: \.self) { index in
                        OptionRow(option: options[index], isMain: index == 0) { trip.select(index) }
                    }
                }
            }
            Button {
                trip.isLive ? trip.end() : trip.start()
            } label: {
                Text(trip.isLive ? "End" : "Start")
                    .font(.headline)
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 14)
                    .background(trip.isLive ? Brand.raised : Brand.textPrimary, in: Capsule())
                    .overlay(Capsule().stroke(trip.isLive ? Brand.hairline : .clear, lineWidth: 1))
                    .foregroundStyle(trip.isLive ? Brand.textPrimary : Brand.ink)
            }
            .buttonStyle(.plain)
        }
    }

    private var minutesAway: Int {
        max(1, Int((route.arriveAt.timeIntervalSinceNow / 60).rounded()))
    }

    private var subtitle: String {
        var parts = ["Arrive \(route.arriveAt.formatted(date: .omitted, time: .shortened))"]
        if route.costCents > 0 {
            parts.append((Double(route.costCents) / 100).formatted(.currency(code: "USD")))
        }
        return parts.joined(separator: " · ")
    }
}

struct OptionRow: View {
    let option: Route
    let isMain: Bool
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            HStack {
                Text(isMain ? "Recommended" : (option.label ?? "Another way"))
                    .font(.subheadline.weight(.medium))
                    .lineLimit(1)
                Spacer()
                Text(option.arriveAt.formatted(date: .omitted, time: .shortened))
                    .font(.subheadline.monospacedDigit())
                    .lineLimit(1)
                    .fixedSize()
                    .foregroundStyle(Brand.textSecondary)
            }
            .padding(.vertical, 10)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .overlay(alignment: .top) {
            Rectangle().fill(Brand.hairline).frame(height: 1)
        }
    }
}

struct LegStrip: View {
    let legs: [Leg]

    var body: some View {
        let shown = legs.filter { $0.isRide || $0.arriveAt.timeIntervalSince($0.departAt) >= 45 }
        let rides = legs.filter(\.isRide)
        ViewThatFits(in: .horizontal) {
            row(shown, names: true)
            row(shown, names: false)
            row(rides.isEmpty ? shown : rides, names: false)
            row(rides.isEmpty ? shown : rides, names: false, minutes: false)
        }
    }

    private func row(_ legs: [Leg], names: Bool, minutes: Bool = true) -> some View {
        HStack(spacing: 6) {
            ForEach(Array(legs.enumerated()), id: \.offset) { index, leg in
                if index > 0 {
                    Image(systemName: "chevron.right")
                        .font(.caption2.weight(.bold))
                        .foregroundStyle(Brand.textFaint)
                }
                chip(for: leg, name: names, minutes: minutes)
            }
        }
        .fixedSize(horizontal: true, vertical: false)
    }

    private func chip(for leg: Leg, name: Bool, minutes: Bool) -> some View {
        HStack(spacing: 4) {
            Image(systemName: ModeSymbol.name(for: leg.mode))
            if name, let line = leg.line {
                Text(LegStrip.shortName(line.name))
            }
            if minutes {
                Text(Duration.short(leg.minutes))
                    .monospacedDigit()
                    .opacity(leg.line == nil ? 1 : 0.7)
            }
        }
        .lineLimit(1)
        .fixedSize()
        .font(.subheadline.weight(.semibold))
        .padding(.horizontal, leg.isRide ? 10 : 2)
        .padding(.vertical, 6)
        .background(leg.isRide ? RootView.color(for: leg).opacity(0.9) : .clear, in: Capsule())
        .foregroundStyle(leg.isRide ? RootView.ink(for: leg) : Brand.textPrimary)
    }

    static func shortName(_ name: String) -> String {
        let words = name.split(separator: " ")
        if let first = words.first, first.count <= 4, first.contains(where: \.isNumber) || first.allSatisfy(\.isUppercase) {
            return String(first)
        }
        let head = name.split(separator: "-").first.map { $0.trimmingCharacters(in: .whitespaces) } ?? name
        return head.count <= 10 ? head : String(head.prefix(9)) + "…"
    }
}

enum Duration {
    static func short(_ minutes: Int) -> String {
        minutes < 60 ? "\(minutes)m" : "\(minutes / 60)h \(minutes % 60)m"
    }

    static func long(_ minutes: Int) -> (String, String) {
        minutes < 60 ? ("\(minutes)", " min") : ("\(minutes / 60)", minutes % 60 == 0 ? " hr" : " hr \(minutes % 60) min")
    }
}

struct CloseButton: View {
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Image(systemName: "xmark")
                .font(.footnote.weight(.bold))
                .frame(width: 30, height: 30)
                .background(Brand.raised, in: Circle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel("Close")
    }
}

extension Color {
    static func luminance(_ hex: String) -> Double {
        let value = UInt64(hex.trimmingCharacters(in: CharacterSet(charactersIn: "#")), radix: 16) ?? 0xFFFFFF
        let red = Double((value >> 16) & 0xFF) / 255
        let green = Double((value >> 8) & 0xFF) / 255
        let blue = Double(value & 0xFF) / 255
        return 0.2126 * red + 0.7152 * green + 0.0722 * blue
    }

}
