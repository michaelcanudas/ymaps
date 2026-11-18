import SwiftUI
import WidgetKit

struct WhereToEntry: TimelineEntry {
    let date: Date
}

struct WhereToProvider: TimelineProvider {
    func placeholder(in context: Context) -> WhereToEntry {
        WhereToEntry(date: .now)
    }

    func getSnapshot(in context: Context, completion: @escaping (WhereToEntry) -> Void) {
        completion(WhereToEntry(date: .now))
    }

    func getTimeline(in context: Context, completion: @escaping (Timeline<WhereToEntry>) -> Void) {
        completion(Timeline(entries: [WhereToEntry(date: .now)], policy: .never))
    }
}

struct WhereToWidgetView: View {
    var body: some View {
        VStack(alignment: .leading) {
            Text("ymaps")
                .font(.caption.weight(.semibold))
                .foregroundStyle(.white.opacity(0.5))
            Spacer()
            HStack(spacing: 8) {
                Image(systemName: "magnifyingglass")
                Text("Where to?")
                    .font(.headline)
            }
            .foregroundStyle(.white)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        .containerBackground(.black, for: .widget)
        .widgetURL(URL(string: "ymaps://search"))
    }
}

struct WhereToWidget: Widget {
    var body: some WidgetConfiguration {
        StaticConfiguration(kind: "WhereToWidget", provider: WhereToProvider()) { _ in
            WhereToWidgetView()
        }
        .configurationDisplayName("Where to")
        .description("Opens ymaps ready for a destination.")
        .supportedFamilies([.systemSmall])
    }
}
