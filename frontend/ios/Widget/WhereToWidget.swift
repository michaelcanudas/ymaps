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
                .foregroundStyle(Brand.textFaint)
            Spacer()
            HStack(spacing: 10) {
                RoundedRectangle(cornerRadius: 1)
                    .fill(Brand.textPrimary)
                    .frame(width: 8, height: 8)
                Text("Where to?")
                    .font(.headline)
            }
            .foregroundStyle(Brand.textPrimary)
            .padding(.horizontal, 12)
            .padding(.vertical, 10)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(Brand.raised, in: RoundedRectangle(cornerRadius: 8, style: .continuous))
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        .containerBackground(Brand.ink, for: .widget)
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
