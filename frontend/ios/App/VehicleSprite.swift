import CoreLocation
import SwiftUI

enum VehicleSprite {
    static func name(for mode: String, heading: Double?) -> String? {
        switch mode {
        case "shuttle", "bus":
            let normalized = ((heading ?? 0).truncatingRemainder(dividingBy: 360) + 360).truncatingRemainder(dividingBy: 360)
            let snapped = Int((normalized / 10).rounded()) * 10 % 360
            return "vehicle-\(mode)-" + String(format: "%03d", snapped)
        case "bike", "veo":
            return "vehicle-\(mode)"
        default:
            return nil
        }
    }

    static func size(for mode: String) -> CGFloat {
        switch mode {
        case "bus": 56
        case "shuttle": 48
        case "bike": 38
        default: 34
        }
    }

    static func bearing(along path: [CLLocationCoordinate2D], at point: CLLocationCoordinate2D) -> Double {
        guard path.count >= 2 else { return 0 }
        var nearest = 0
        var best = Double.infinity
        for (index, coordinate) in path.enumerated() {
            let distance = pow(coordinate.latitude - point.latitude, 2) + pow(coordinate.longitude - point.longitude, 2)
            if distance < best {
                best = distance
                nearest = index
            }
        }
        let from = nearest < path.count - 1 ? path[nearest] : path[nearest - 1]
        let to = nearest < path.count - 1 ? path[nearest + 1] : path[nearest]
        let dx = (to.longitude - from.longitude) * cos(from.latitude * .pi / 180)
        let dy = to.latitude - from.latitude
        return (atan2(dx, dy) * 180 / .pi + 360).truncatingRemainder(dividingBy: 360)
    }
}

struct ModeIcon: View {
    let mode: String
    var size: CGFloat = 20

    var body: some View {
        if ["shuttle", "bus", "bike", "veo"].contains(mode) {
            Image("mode-\(mode)")
                .resizable()
                .interpolation(.high)
                .frame(width: size, height: size)
        } else {
            Image(systemName: ModeSymbol.name(for: mode))
                .font(.system(size: size * 0.62, weight: .semibold))
                .foregroundStyle(Brand.color(forMode: mode))
                .frame(width: size, height: size)
        }
    }
}
