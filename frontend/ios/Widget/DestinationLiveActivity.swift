import ActivityKit
import SwiftUI
import WidgetKit

struct DestinationLiveActivity: Widget {
    var body: some WidgetConfiguration {
        ActivityConfiguration(for: DestinationActivityAttributes.self) { context in
            HStack(spacing: 12) {
                Image(systemName: context.state.symbol)
                    .font(.title3)
                VStack(alignment: .leading, spacing: 2) {
                    Text(context.attributes.destination)
                        .font(.headline)
                    Text(context.state.step)
                        .font(.subheadline)
                        .foregroundStyle(.white.opacity(0.6))
                        .lineLimit(1)
                }
                Spacer()
                VStack(alignment: .trailing, spacing: 2) {
                    Text(timerInterval: Date.now...max(Date.now, context.state.arriveAt), countsDown: true, showsHours: false)
                        .font(.title3.monospacedDigit().weight(.semibold))
                        .multilineTextAlignment(.trailing)
                        .frame(width: 72, alignment: .trailing)
                    Text(context.state.arriveAt, style: .time)
                        .font(.caption)
                        .foregroundStyle(.white.opacity(0.6))
                }
            }
            .padding(16)
            .foregroundStyle(.white)
            .activityBackgroundTint(.black)
            .activitySystemActionForegroundColor(.white)
        } dynamicIsland: { context in
            DynamicIsland {
                DynamicIslandExpandedRegion(.leading) {
                    Label(context.attributes.destination, systemImage: context.state.symbol)
                        .font(.headline)
                        .lineLimit(1)
                }
                DynamicIslandExpandedRegion(.trailing) {
                    Text(context.state.arriveAt, style: .time)
                        .font(.headline)
                }
                DynamicIslandExpandedRegion(.bottom) {
                    Text(context.state.step)
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            } compactLeading: {
                Image(systemName: context.state.symbol)
            } compactTrailing: {
                Text(timerInterval: Date.now...max(Date.now, context.state.arriveAt), countsDown: true, showsHours: false)
                    .monospacedDigit()
                    .frame(width: 44)
            } minimal: {
                Image(systemName: context.state.symbol)
            }
        }
    }
}
