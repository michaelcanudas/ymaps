import MapKit
import SwiftUI

struct RootView: View {
    @Environment(Trip.self) private var trip
    @State private var camera: MapCameraPosition = .userLocation(fallback: .region(RootView.campus))
    @Binding var searchRequest: Int
    #if LIVE_SHUTTLES
    @State private var shuttles = LiveShuttles()
    #endif

    static let campus = MKCoordinateRegion(
        center: CLLocationCoordinate2D(latitude: 41.3105, longitude: -72.9260),
        span: MKCoordinateSpan(latitudeDelta: 0.018, longitudeDelta: 0.018)
    )

    var body: some View {
        map
            .ignoresSafeArea()
            .safeAreaInset(edge: .bottom) {
                TripPanel(searchRequest: searchRequest)
                    .padding(.horizontal, 12)
                    .padding(.bottom, 4)
            }
            .background(.black)
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
                    if let line = leg.line {
                        MapPolyline(coordinates: Polyline.decode(line.path))
                            .stroke(Color(hex: line.color).opacity(0.25), lineWidth: 3)
                    }
                    MapPolyline(coordinates: Polyline.decode(leg.path))
                        .stroke(RootView.color(for: leg), style: RootView.stroke(for: leg))
                }
                ForEach(Array(route.legs.dropFirst().enumerated()), id: \.offset) { _, leg in
                    Annotation("", coordinate: leg.from.coordinate.location) {
                        Circle()
                            .fill(.black)
                            .stroke(.white, lineWidth: 3)
                            .frame(width: 12, height: 12)
                    }
                }
                Annotation("", coordinate: route.legs.last!.to.coordinate.location) {
                    RoundedRectangle(cornerRadius: 3)
                        .fill(.white)
                        .frame(width: 14, height: 14)
                }
            }
        }
        .mapStyle(.standard(emphasis: .muted, pointsOfInterest: .excludingAll))
        .mapControlVisibility(.hidden)
        .ignoresSafeArea(.keyboard)
    }

    static func color(for leg: Leg) -> Color {
        switch leg.mode {
        case "walk": .white.opacity(0.75)
        case "bike": Color(hex: "34C759")
        case "veo": Color(hex: "AF52DE")
        default: leg.line.map { Color(hex: $0.color) } ?? .white
        }
    }

    static func stroke(for leg: Leg) -> StrokeStyle {
        leg.mode == "walk"
            ? StrokeStyle(lineWidth: 4, lineCap: .round, dash: [1, 8])
            : StrokeStyle(lineWidth: 6, lineCap: .round, lineJoin: .round)
    }

    static func framing(_ route: Route) -> MapCameraPosition {
        let points = route.legs.flatMap { Polyline.decode($0.path) }
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
            .foregroundStyle(.white)
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
                    Text(destination).font(.headline)
                    Spacer()
                    ProgressView().tint(.white)
                }
            }
        case .failed(let message):
            VStack(alignment: .leading, spacing: 8) {
                Text(message)
                    .font(.subheadline)
                    .foregroundStyle(.white.opacity(0.6))
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
            .background(.black, in: RoundedRectangle(cornerRadius: 24, style: .continuous))
    }

    private var searchBar: some View {
        HStack(spacing: 10) {
            Image(systemName: "magnifyingglass")
                .foregroundStyle(.white.opacity(0.5))
            TextField("", text: $query, prompt: Text("Where to?").foregroundStyle(.white.opacity(0.5)))
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
                        .foregroundStyle(.white.opacity(0.4))
                }
                .buttonStyle(.plain)
            }
            if searching {
                Button("Cancel") {
                    searching = false
                    query = ""
                }
                .font(.subheadline.weight(.medium))
                .foregroundStyle(.white.opacity(0.7))
                .buttonStyle(.plain)
                .transition(.opacity)
            }
        }
        .padding(.horizontal, 18)
        .frame(height: 56)
        .background(.black, in: Capsule())
    }

    private func choices(_ destination: String, _ places: [Place]) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Text("Which \(destination)?").font(.headline)
                Spacer()
                CloseButton { trip.clear() }
            }
            .padding(.bottom, 6)
            ForEach(places) { place in
                Button {
                    trip.go(to: destination, placeId: place.id)
                } label: {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(place.name).font(.body.weight(.medium))
                        if let address = place.address {
                            Text(address)
                                .font(.caption)
                                .foregroundStyle(.white.opacity(0.5))
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
                (Text("\(minutesAway)").font(.system(size: 40, weight: .bold, design: .rounded))
                    + Text(" min").font(.title3.weight(.semibold)))
                Spacer()
                CloseButton { trip.clear() }
            }
            VStack(alignment: .leading, spacing: 2) {
                Text(route.place.entryway?.name ?? route.place.name)
                    .font(.headline)
                    .lineLimit(1)
                Text(subtitle)
                    .font(.subheadline)
                    .foregroundStyle(.white.opacity(0.55))
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
                    .background(trip.isLive ? Color.white.opacity(0.12) : .white, in: Capsule())
                    .foregroundStyle(trip.isLive ? .white : .black)
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
                    .foregroundStyle(.white.opacity(0.5))
            }
            .padding(.vertical, 10)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .overlay(alignment: .top) {
            Rectangle().fill(.white.opacity(0.08)).frame(height: 1)
        }
    }
}

struct LegStrip: View {
    let legs: [Leg]

    var body: some View {
        let shown = legs.filter { $0.isRide || $0.arriveAt.timeIntervalSince($0.departAt) >= 45 }
        HStack(spacing: 6) {
            ForEach(Array(shown.enumerated()), id: \.offset) { index, leg in
                if index > 0 {
                    Image(systemName: "chevron.right")
                        .font(.caption2.weight(.bold))
                        .foregroundStyle(.white.opacity(0.3))
                }
                chip(for: leg)
            }
        }
    }

    private func chip(for leg: Leg) -> some View {
        HStack(spacing: 4) {
            Image(systemName: ModeSymbol.name(for: leg.mode))
            if let line = leg.line {
                Text(shortName(line.name)).lineLimit(1)
            }
            Text("\(leg.minutes)m")
                .monospacedDigit()
                .opacity(leg.line == nil ? 1 : 0.7)
        }
        .font(.subheadline.weight(.semibold))
        .padding(.horizontal, leg.isRide ? 10 : 2)
        .padding(.vertical, 6)
        .background(leg.isRide ? RootView.color(for: leg).opacity(0.9) : .clear, in: Capsule())
        .foregroundStyle(leg.isRide ? .black : .white.opacity(0.8))
    }

    private func shortName(_ name: String) -> String {
        if let first = name.split(separator: " ").first, first.allSatisfy(\.isNumber) {
            return String(first)
        }
        return String(name.split(separator: "-").first ?? Substring(name)).trimmingCharacters(in: .whitespaces)
    }
}

struct CloseButton: View {
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Image(systemName: "xmark")
                .font(.footnote.weight(.bold))
                .frame(width: 30, height: 30)
                .background(.white.opacity(0.12), in: Circle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel("Close")
    }
}

extension Color {
    init(hex: String) {
        let value = UInt64(hex.trimmingCharacters(in: CharacterSet(charactersIn: "#")), radix: 16) ?? 0xFFFFFF
        self.init(
            red: Double((value >> 16) & 0xFF) / 255,
            green: Double((value >> 8) & 0xFF) / 255,
            blue: Double(value & 0xFF) / 255
        )
    }
}
