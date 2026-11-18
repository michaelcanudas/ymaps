import SwiftUI

@main
struct YmapsApp: App {
    @State private var trip = Trip()
    @State private var searchRequest = 0

    var body: some Scene {
        WindowGroup {
            RootView(searchRequest: $searchRequest)
                .environment(trip)
                .preferredColorScheme(.dark)
                .onOpenURL { url in
                    if url.host() == "search" {
                        searchRequest += 1
                    }
                }
        }
    }
}
