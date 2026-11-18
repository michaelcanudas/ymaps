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

    static let world = [
        CLLocationCoordinate2D(latitude: 85, longitude: -179.9),
        CLLocationCoordinate2D(latitude: 85, longitude: 179.9),
        CLLocationCoordinate2D(latitude: -85, longitude: 179.9),
        CLLocationCoordinate2D(latitude: -85, longitude: -179.9),
    ]

    var body: some View {
        ZStack(alignment: .bottom) {
            map
            card
                .padding(.horizontal, 12)
                .padding(.bottom, 8)
        }
        .background(Brand.mapBackground)
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
            MapPolygon(coordinates: RootView.world)
                .foregroundStyle(Brand.mapBackground.opacity(0.5))
                .mapOverlayLevel(level: .aboveRoads)
            UserAnnotation()
            if let route = trip.route {
                ForEach(Array(route.legs.enumerated()), id: \.offset) { _, leg in
                    if let line = leg.line {
                        MapPolyline(coordinates: Polyline.decode(line.path))
                            .stroke(Color(hex: line.color).opacity(0.22), lineWidth: 3)
                    }
                    if leg.isRide {
                        MapPolyline(coordinates: Polyline.decode(leg.path))
                            .stroke(Brand.ink.opacity(0.9), style: StrokeStyle(lineWidth: 9, lineCap: .round, lineJoin: .round))
                    }
                    MapPolyline(coordinates: Polyline.decode(leg.path))
                        .stroke(RootView.color(for: leg), style: RootView.stroke(for: leg))
                }
                ForEach(Array(route.legs.dropFirst().enumerated()), id: \.offset) { _, leg in
                    Annotation("", coordinate: leg.from.coordinate.location, anchor: .center) {
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
                Annotation(route.place.name, coordinate: route.legs.last!.to.coordinate.location, anchor: .center) {
                    RoundedRectangle(cornerRadius: 2)
                        .fill(Brand.textPrimary)
                        .frame(width: 16, height: 16)
                        .overlay(Rectangle().fill(Brand.ink).frame(width: 6, height: 6))
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
                    ProgressView().tint(Brand.textPrimary)
                }
            case .choosing(let destination, let places):
                choices(destination, places)
            case .failed(let message):
                Text(message)
                    .font(.subheadline)
                    .foregroundStyle(Brand.textSecondary)
                searchField
            case .route(let options, let index):
                RouteCard(route: options[index], options: options, selected: index)
            }
        }
        .padding(18)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Brand.panel.opacity(0.97), in: RoundedRectangle(cornerRadius: 16, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 16, style: .continuous).stroke(Brand.hairline, lineWidth: 1))
        .shadow(color: .black.opacity(0.35), radius: 24, y: 8)
        .foregroundStyle(Brand.textPrimary)
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
        HStack(spacing: 12) {
            RoundedRectangle(cornerRadius: 1)
                .fill(Brand.textPrimary)
                .frame(width: 8, height: 8)
            TextField("", text: $query, prompt: Text("Where to?").foregroundStyle(Brand.textSecondary))
                .font(.title3.weight(.semibold))
                .focused($searching)
                .submitLabel(.go)
                .autocorrectionDisabled()
                .onSubmit {
                    searching = false
                    trip.go(to: query)
                }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 12)
        .background(Brand.raised, in: RoundedRectangle(cornerRadius: 8, style: .continuous))
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

    static func color(for leg: Leg) -> Color {
        Brand.color(forMode: leg.mode)
    }

    static func stroke(for leg: Leg) -> StrokeStyle {
        leg.mode == "walk"
            ? StrokeStyle(lineWidth: 4, lineCap: .round, dash: [1, 8])
            : StrokeStyle(lineWidth: 5, lineCap: .round, lineJoin: .round)
    }

    static func vehicleMarker(for leg: Leg) -> (coordinate: CLLocationCoordinate2D, sprite: String)? {
        switch leg.mode {
        case "shuttle", "bus":
            guard let vehicle = leg.vehicle else { return nil }
            let heading = vehicle.heading ?? leg.line.map {
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
                (Text("\(minutesAway)").font(.system(size: 40, weight: .bold))
                    + Text(" min").font(.title3.weight(.semibold)))
                Spacer()
                if trip.isLive {
                    Text("LIVE")
                        .font(.caption2.weight(.semibold))
                        .tracking(0.5)
                        .foregroundStyle(Brand.accent)
                }
                CloseButton { trip.clear() }
            }
            VStack(alignment: .leading, spacing: 2) {
                Text(route.place.entryway?.name ?? route.place.name)
                    .font(.headline)
                    .lineLimit(1)
                Text(subtitle)
                    .font(.subheadline)
                    .foregroundStyle(Brand.textSecondary)
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
                    .foregroundStyle(isMain ? Brand.textPrimary : Brand.textSecondary)
                Text(isMain ? "Recommended" : (option.label ?? "Another way"))
                    .font(.subheadline.weight(.medium))
                    .lineLimit(1)
                Spacer()
                HStack(spacing: 2) {
                    ForEach(Array(option.legs.filter(\.isRide).enumerated()), id: \.offset) { _, leg in
                        ModeIcon(mode: leg.mode, size: 20)
                    }
                }
                Text(option.arriveAt.formatted(date: .omitted, time: .shortened))
                    .font(.subheadline.monospacedDigit())
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
                        .foregroundStyle(Brand.textFaint)
                }
                chip(for: leg)
            }
        }
    }

    private func chip(for leg: Leg) -> some View {
        let color = RootView.color(for: leg)
        return HStack(spacing: 4) {
            ModeIcon(mode: leg.mode, size: leg.isRide ? 22 : 16)
            if let line = leg.line {
                Text(shortName(line.name)).lineLimit(1)
            }
            Text("\(leg.minutes)m")
                .monospacedDigit()
                .opacity(leg.line == nil ? 1 : 0.75)
        }
        .font(.subheadline.weight(.semibold))
        .padding(.horizontal, leg.isRide ? 9 : 2)
        .padding(.vertical, 4)
        .background(leg.isRide ? color.opacity(0.13) : .clear, in: Capsule())
        .overlay(Capsule().stroke(leg.isRide ? color.opacity(0.4) : .clear, lineWidth: 1))
        .foregroundStyle(leg.isRide ? color : Brand.textSecondary)
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
                .foregroundStyle(Brand.textSecondary)
                .frame(width: 30, height: 30)
                .background(Brand.raised, in: Circle())
        }
        .buttonStyle(.plain)
    }
}
