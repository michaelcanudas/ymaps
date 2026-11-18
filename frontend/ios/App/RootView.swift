import MapKit
import SwiftUI

struct RootView: View {
    @Environment(Trip.self) private var trip
    @State private var camera: MapCameraPosition = .userLocation(fallback: .region(RootView.campus))
    @State private var query = ""
    @FocusState private var searching: Bool
    @Binding var searchRequest: Int

    static let campus = MKCoordinateRegion(
        center: CLLocationCoordinate2D(latitude: 41.3105, longitude: -72.9260),
        span: MKCoordinateSpan(latitudeDelta: 0.018, longitudeDelta: 0.018)
    )

    var body: some View {
        ZStack(alignment: .bottom) {
            map
            card
                .padding(.horizontal, 12)
                .padding(.bottom, 8)
        }
        .background(.black)
        .onChange(of: trip.route) { _, route in
            guard let route else { return }
            withAnimation(.easeInOut(duration: 0.5)) { camera = RootView.framing(route) }
        }
        .onChange(of: phaseKey) { _, key in
            if key == "idle" { query = "" }
        }
        .onChange(of: searchRequest) {
            trip.clear()
            searching = true
        }
    }

    private var map: some View {
        Map(position: $camera) {
            UserAnnotation()
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
                Annotation(route.place.name, coordinate: route.legs.last!.to.coordinate.location) {
                    RoundedRectangle(cornerRadius: 3)
                        .fill(.white)
                        .frame(width: 14, height: 14)
                }
            }
        }
        .mapStyle(.standard(emphasis: .muted, pointsOfInterest: .excludingAll))
        .mapControlVisibility(.hidden)
        .ignoresSafeArea()
    }

    @ViewBuilder
    private var card: some View {
        VStack(alignment: .leading, spacing: 14) {
            switch trip.phase {
            case .idle:
                searchField
            case .loading(let destination):
                HStack {
                    Text(destination).font(.headline)
                    Spacer()
                    ProgressView().tint(.white)
                }
            case .choosing(let destination, let places):
                choices(destination, places)
            case .failed(let message):
                Text(message)
                    .font(.subheadline)
                    .foregroundStyle(.white.opacity(0.6))
                searchField
            case .route(let options, let index):
                RouteCard(route: options[index], options: options, selected: index)
            }
        }
        .padding(18)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.black, in: RoundedRectangle(cornerRadius: 24, style: .continuous))
        .foregroundStyle(.white)
        .animation(.easeInOut(duration: 0.2), value: phaseKey)
    }

    private var phaseKey: String {
        switch trip.phase {
        case .idle: "idle"
        case .loading: "loading"
        case .choosing: "choosing"
        case .failed: "failed"
        case .route(let options, let index): options[index].id + options[index].arriveAt.description
        }
    }

    private var searchField: some View {
        HStack(spacing: 10) {
            Image(systemName: "magnifyingglass")
                .foregroundStyle(.white.opacity(0.5))
            TextField("", text: $query, prompt: Text("Where to?").foregroundStyle(.white.opacity(0.5)))
                .font(.title3.weight(.semibold))
                .focused($searching)
                .submitLabel(.go)
                .autocorrectionDisabled()
                .onSubmit {
                    searching = false
                    trip.go(to: query)
                }
        }
        .padding(.vertical, 4)
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
                    ForEach(Array(options.enumerated()), id: \.offset) { index, option in
                        if index != selected {
                            OptionRow(option: option, isMain: index == 0) { trip.select(index) }
                        }
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
        if let first = route.legs.first(where: { $0.isRide }) {
            parts.append(Trip.instruction(for: first))
        }
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
            HStack(spacing: 10) {
                Image(systemName: symbol)
                    .font(.footnote.weight(.semibold))
                    .frame(width: 18)
                    .foregroundStyle(.white.opacity(0.6))
                Text(isMain ? "Recommended" : (option.label ?? "Another way"))
                    .font(.subheadline.weight(.medium))
                    .lineLimit(1)
                Spacer()
                HStack(spacing: 3) {
                    ForEach(Array(option.legs.filter(\.isRide).enumerated()), id: \.offset) { _, leg in
                        Image(systemName: ModeSymbol.name(for: leg.mode))
                    }
                }
                .font(.caption)
                .foregroundStyle(.white.opacity(0.5))
                Text(option.arriveAt.formatted(date: .omitted, time: .shortened))
                    .font(.subheadline.monospacedDigit())
                    .foregroundStyle(.white.opacity(0.6))
            }
            .padding(.vertical, 10)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .overlay(alignment: .top) {
            Rectangle().fill(.white.opacity(0.08)).frame(height: 1)
        }
    }

    private var symbol: String {
        if isMain { return "star.fill" }
        switch option.kind {
        case "free": return "tag.fill"
        case "faster": return "bolt.fill"
        case "lessWalking": return "figure.walk"
        case "cheaper": return "dollarsign"
        default: return "arrow.triangle.branch"
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
