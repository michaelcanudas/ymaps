import ActivityKit
import CoreLocation
import Foundation
import Observation

@MainActor
@Observable
final class Trip: NSObject, CLLocationManagerDelegate {
    enum Phase {
        case idle
        case loading(String)
        case choosing(String, [Place])
        case route([Route], Int)
        case failed(String)
    }

    var phase: Phase = .idle
    var isLive = false
    var here = Coordinate(lat: 41.3083, lon: -72.9279)

    private let api = API.configured
    private let locationManager = CLLocationManager()
    private var activity: Activity<DestinationActivityAttributes>?
    private var refreshTask: Task<Void, Never>?

    override init() {
        super.init()
        locationManager.delegate = self
        locationManager.desiredAccuracy = kCLLocationAccuracyNearestTenMeters
        locationManager.requestWhenInUseAuthorization()
        locationManager.startUpdatingLocation()
    }

    var route: Route? {
        if case .route(let options, let index) = phase { return options[index] }
        return nil
    }

    var options: [Route] {
        if case .route(let options, _) = phase { return options }
        return []
    }

    func select(_ index: Int) {
        guard case .route(let options, _) = phase, options.indices.contains(index) else { return }
        phase = .route(options, index)
    }

    func go(to destination: String, placeId: String? = nil) {
        let trimmed = destination.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        end()
        phase = .loading(trimmed)
        Task {
            do {
                switch try await api.plan(from: here, to: trimmed, placeId: placeId) {
                case .route(let route): phase = .route([route] + (route.alternatives ?? []), 0)
                case .choose(let places): phase = .choosing(trimmed, places)
                case .notFound: phase = .failed("Couldn't find \(trimmed).")
                }
            } catch {
                phase = .failed(error.localizedDescription)
            }
        }
    }

    func clear() {
        end()
        phase = .idle
    }

    func start() {
        guard let route else { return }
        isLive = true
        if ActivityAuthorizationInfo().areActivitiesEnabled {
            let attributes = DestinationActivityAttributes(destination: route.place.name)
            activity = try? Activity.request(attributes: attributes, content: content(for: route))
        }
        refreshTask = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(60))
                await self?.refresh()
            }
        }
    }

    func end() {
        isLive = false
        refreshTask?.cancel()
        refreshTask = nil
        if let activity {
            self.activity = nil
            Task { await activity.end(nil, dismissalPolicy: .immediate) }
        }
    }

    private func refresh() async {
        guard case .route(var options, let index) = phase,
              let fresh = try? await api.refresh(options[index].id, from: here) else { return }
        options[index] = fresh
        phase = .route(options, index)
        await activity?.update(content(for: fresh))
    }

    private func content(for route: Route) -> ActivityContent<DestinationActivityAttributes.ContentState> {
        let next = route.legs.first(where: { $0.arriveAt > .now }) ?? route.legs.last!
        let state = DestinationActivityAttributes.ContentState(
            arriveAt: route.arriveAt,
            step: Trip.instruction(for: next),
            symbol: ModeSymbol.name(for: next.mode)
        )
        return ActivityContent(state: state, staleDate: route.arriveAt.addingTimeInterval(600))
    }

    static func instruction(for leg: Leg) -> String {
        let destination = leg.to.name ?? "destination"
        switch leg.mode {
        case "walk":
            return "Walk to \(destination)"
        case "shuttle", "bus":
            let name = leg.line?.name ?? "Bus"
            return "\(name) at \(leg.departAt.formatted(date: .omitted, time: .shortened))"
        case "bike":
            return "Bike to \(destination)"
        case "veo":
            return "Veo to \(destination)"
        default:
            return destination
        }
    }

    nonisolated func locationManager(_ manager: CLLocationManager, didUpdateLocations locations: [CLLocation]) {
        guard let last = locations.last else { return }
        let coordinate = Coordinate(lat: last.coordinate.latitude, lon: last.coordinate.longitude)
        Task { @MainActor in self.here = coordinate }
    }

    nonisolated func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        manager.startUpdatingLocation()
    }
}
