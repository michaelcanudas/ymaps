enum ModeSymbol {
    static func name(for mode: String) -> String {
        switch mode {
        case "walk": "figure.walk"
        case "shuttle": "bus.fill"
        case "bus": "bus.doubledecker.fill"
        case "bike": "bicycle"
        case "veo": "scooter"
        default: "arrow.right"
        }
    }
}
