import CoreLocation
import Foundation

struct Coordinate: Codable, Hashable {
    let lat: Double
    let lon: Double

    var location: CLLocationCoordinate2D {
        CLLocationCoordinate2D(latitude: lat, longitude: lon)
    }
}

struct Entryway: Codable, Hashable {
    let name: String
    let coordinate: Coordinate
}

struct Place: Codable, Hashable, Identifiable {
    let id: String
    let name: String
    let address: String?
    let coordinate: Coordinate
    let entryway: Entryway?
}

struct Point: Codable, Hashable {
    let id: String?
    let name: String?
    let coordinate: Coordinate
}

struct Line: Codable, Hashable {
    let id: String
    let name: String
    let color: String
    let path: String
}

struct Vehicle: Codable, Hashable {
    let id: String
    let name: String?
    let coordinate: Coordinate
}

struct Leg: Codable, Hashable {
    let mode: String
    let from: Point
    let to: Point
    let departAt: Date
    let arriveAt: Date
    let costCents: Int
    let path: String
    let line: Line?
    let headsign: String?
    let stopCount: Int?
    let vehicle: Vehicle?
    let form: String?

    var minutes: Int {
        max(1, Int((arriveAt.timeIntervalSince(departAt) / 60).rounded()))
    }

    var isRide: Bool {
        mode != "walk"
    }
}

struct Route: Codable, Hashable {
    let id: String
    let place: Place
    let departAt: Date
    let arriveAt: Date
    let costCents: Int
    let legs: [Leg]
}

struct Unresolved: Codable {
    let destination: String
    let places: [Place]
}

struct RouteRequest: Encodable {
    let origin: Coordinate
    let destination: String
    let placeId: String?
}

enum PlanResult {
    case route(Route)
    case choose([Place])
    case notFound
}

enum APIError: LocalizedError {
    case server(String)

    var errorDescription: String? {
        switch self {
        case .server(let message): message
        }
    }
}

struct API {
    let baseURL: URL

    static var configured: API {
        let override = UserDefaults.standard.string(forKey: "server")
        let bundled = Bundle.main.object(forInfoDictionaryKey: "YmapsServerURL") as? String
        let url = URL(string: override ?? bundled ?? "http://localhost:8080")!
        return API(baseURL: url)
    }

    func plan(from origin: Coordinate, to destination: String, placeId: String? = nil) async throws -> PlanResult {
        var request = URLRequest(url: baseURL.appending(path: "v1/routes"))
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try JSONEncoder().encode(RouteRequest(origin: origin, destination: destination, placeId: placeId))
        let (data, response) = try await URLSession.shared.data(for: request)
        let status = (response as? HTTPURLResponse)?.statusCode ?? 0
        switch status {
        case 200:
            return .route(try Self.decoder.decode(Route.self, from: data))
        case 422:
            let unresolved = try Self.decoder.decode(Unresolved.self, from: data)
            return unresolved.places.isEmpty ? .notFound : .choose(unresolved.places)
        default:
            throw APIError.server("Something went wrong (\(status)).")
        }
    }

    func refresh(_ routeId: String, from origin: Coordinate) async throws -> Route? {
        var components = URLComponents(url: baseURL.appending(path: "v1/routes/\(routeId)"), resolvingAgainstBaseURL: false)!
        components.queryItems = [
            URLQueryItem(name: "lat", value: String(origin.lat)),
            URLQueryItem(name: "lon", value: String(origin.lon)),
        ]
        let (data, response) = try await URLSession.shared.data(from: components.url!)
        guard (response as? HTTPURLResponse)?.statusCode == 200 else { return nil }
        return try Self.decoder.decode(Route.self, from: data)
    }

    static let decoder: JSONDecoder = {
        let decoder = JSONDecoder()
        let precise = ISO8601DateFormatter()
        precise.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        let plain = ISO8601DateFormatter()
        decoder.dateDecodingStrategy = .custom { decoder in
            let text = try decoder.singleValueContainer().decode(String.self)
            if let date = precise.date(from: text) ?? plain.date(from: text) {
                return date
            }
            throw DecodingError.dataCorrupted(.init(codingPath: decoder.codingPath, debugDescription: text))
        }
        return decoder
    }()
}

enum Polyline {
    static func decode(_ encoded: String) -> [CLLocationCoordinate2D] {
        var coordinates: [CLLocationCoordinate2D] = []
        let bytes = Array(encoded.utf8)
        var index = 0
        var lat = 0
        var lon = 0
        while index < bytes.count {
            lat += next(bytes, &index)
            lon += next(bytes, &index)
            coordinates.append(CLLocationCoordinate2D(latitude: Double(lat) / 1e5, longitude: Double(lon) / 1e5))
        }
        return coordinates
    }

    private static func next(_ bytes: [UInt8], _ index: inout Int) -> Int {
        var result = 0
        var shift = 0
        while index < bytes.count {
            let chunk = Int(bytes[index]) - 63
            index += 1
            result |= (chunk & 0x1f) << shift
            shift += 5
            if chunk < 0x20 { break }
        }
        return (result & 1) != 0 ? ~(result >> 1) : result >> 1
    }
}
