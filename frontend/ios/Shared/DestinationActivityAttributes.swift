import ActivityKit
import Foundation

struct DestinationActivityAttributes: ActivityAttributes {
    struct ContentState: Codable, Hashable {
        var arriveAt: Date
        var step: String
        var symbol: String
        var mode: String
    }

    var destination: String
}
