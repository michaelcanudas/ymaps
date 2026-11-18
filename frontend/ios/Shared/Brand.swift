import SwiftUI

enum Brand {
    static let ink = Color(hex: "070F1F")
    static let mapBackground = Color(hex: "0E1A2E")
    static let panel = Color(hex: "101C30")
    static let raised = Color(hex: "16243B")
    static let hairline = Color(hex: "1F3150")
    static let textPrimary = Color(hex: "E6ECF5")
    static let textSecondary = Color(hex: "8A9BB5")
    static let textFaint = Color(hex: "5D6F8C")
    static let accent = Color(hex: "6EA8FF")

    static func color(forMode mode: String) -> Color {
        switch mode {
        case "walk": Color(hex: "8CC8FF")
        case "bike": Color(hex: "5ED39B")
        case "veo": Color(hex: "F59AC0")
        case "shuttle": Color(hex: "AFA3FF")
        case "bus": Color(hex: "F2B66D")
        default: textPrimary
        }
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
