import { LayoutChangeEvent, Linking, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import { Leg, Place, Route } from '../api/types';
import { RouteState } from '../state/useTrip';
import { modeAppearance } from '../theme/modes';
import { fontFamily, palette, radius } from '../theme/palette';
import { isHovered } from './isHovered';
import { clockLabel, costLabel, legSeconds, minutesLabel, secondsBetween } from './legTiming';
import { LegStrip } from './LegStrip';
import { ModeGlyph } from './ModeGlyph';

type RouteCardProps = {
  routeState: RouteState;
  isLocationUnavailable: boolean;
  isWide: boolean;
  onChoosePlace: (place: Place) => void;
  onLayout: (event: LayoutChangeEvent) => void;
};

function stopsLabel(stopCount: number) {
  return stopCount === 1 ? '1 stop' : `${stopCount} stops`;
}

function legText(leg: Leg): { title: string; detail: string } {
  const toName = leg.to.name ?? 'your destination';
  const fromName = leg.from.name ?? 'the stop';
  switch (leg.mode) {
    case 'walk':
      return { title: 'Walk', detail: `to ${toName}` };
    case 'shuttle':
      return {
        title: `${leg.line.name} shuttle`,
        detail: `${fromName} at ${clockLabel(leg.departAt)} · ${stopsLabel(leg.stopCount)}`,
      };
    case 'bus':
      return {
        title: `${leg.line.name} to ${leg.headsign}`,
        detail: `${fromName} at ${clockLabel(leg.departAt)} · ${stopsLabel(leg.stopCount)}`,
      };
    case 'bike': {
      const bikes = leg.bikesAvailable === undefined ? '' : ` · ${leg.bikesAvailable} ${leg.bikesAvailable === 1 ? 'bike' : 'bikes'}`;
      return { title: modeAppearance.bike.label, detail: `${fromName} to ${toName}${bikes}` };
    }
    case 'veo':
      return { title: `Veo ${leg.form}`, detail: `to ${toName}` };
  }
}

function LegRow({ leg }: { leg: Leg }) {
  const { title, detail } = legText(leg);
  const rentalUri = leg.mode === 'veo' ? leg.rentalUri : undefined;
  return (
    <View style={styles.legRow}>
      <View style={styles.legIcon}>
        <ModeGlyph mode={leg.mode} size={14} />
      </View>
      <View style={styles.legText}>
        <Text style={styles.legTitle} numberOfLines={1}>
          {title}
        </Text>
        <Text style={styles.legDetail} numberOfLines={2}>
          {detail}
        </Text>
        {rentalUri ? (
          <Pressable onPress={() => Linking.openURL(rentalUri)} style={styles.linkButton}>
            <Text style={styles.linkText}>Open in Veo</Text>
          </Pressable>
        ) : null}
      </View>
      <Text style={styles.legDuration}>{minutesLabel(legSeconds(leg))}</Text>
    </View>
  );
}

function RouteSummary({ route, isRefreshing }: { route: Route; isRefreshing: boolean }) {
  const lateMinutes = Math.round(secondsBetween(route.arriveAt, route.arriveAtLate) / 60);
  return (
    <View style={isRefreshing && styles.refreshing}>
      <View style={styles.summaryRow}>
        <Text style={styles.duration}>{minutesLabel(secondsBetween(route.departAt, route.arriveAt))}</Text>
        <Text style={styles.arrival}>Arrive {clockLabel(route.arriveAt)}</Text>
      </View>
      <Text style={styles.subline} numberOfLines={1}>
        {route.place.name}
        {lateMinutes >= 1 ? ` · up to ${lateMinutes} min later` : ''} · {costLabel(route.costCents)}
      </Text>
      {route.place.entryway ? (
        <Text style={styles.entryway} numberOfLines={1}>
          Enter at {route.place.entryway.name}
        </Text>
      ) : null}
      <LegStrip legs={route.legs} />
      <View style={styles.legs}>
        {route.legs.map((leg, index) => (
          <LegRow key={index} leg={leg} />
        ))}
      </View>
    </View>
  );
}

function PlaceChoices({ places, onChoose }: { places: Place[]; onChoose: (place: Place) => void }) {
  return (
    <View>
      <Text style={styles.prompt}>Which one?</Text>
      <View style={styles.choices}>
        {places.map((place) => (
          <Pressable
            key={place.id}
            onPress={() => onChoose(place)}
            style={(state) => [styles.choice, isHovered(state) && styles.choiceHovered]}
          >
            <Text style={styles.choiceName} numberOfLines={1}>
              {place.name}
            </Text>
            {place.address && place.address !== place.name ? (
              <Text style={styles.choiceAddress} numberOfLines={1}>
                {place.address}
              </Text>
            ) : null}
          </Pressable>
        ))}
      </View>
    </View>
  );
}

function Skeleton() {
  return (
    <View>
      <View style={[styles.skeletonBar, { width: 96, height: 24 }]} />
      <View style={[styles.skeletonBar, { width: '100%', height: 6, marginTop: 14 }]} />
      <View style={[styles.skeletonBar, { width: '70%', height: 12, marginTop: 16 }]} />
      <View style={[styles.skeletonBar, { width: '55%', height: 12, marginTop: 10 }]} />
    </View>
  );
}

export function RouteCard({ routeState, isLocationUnavailable, isWide, onChoosePlace, onLayout }: RouteCardProps) {
  const content = (() => {
    switch (routeState.status) {
      case 'idle':
        return null;
      case 'needsOrigin':
        return (
          <Text style={styles.message}>
            {isLocationUnavailable ? 'Location is off. ' : 'Finding your location… '}
            <Text style={styles.messageStrong}>Tap the map to set your start.</Text>
          </Text>
        );
      case 'loading':
        return routeState.previous ? <RouteSummary route={routeState.previous} isRefreshing /> : <Skeleton />;
      case 'choosing':
        return <PlaceChoices places={routeState.places} onChoose={onChoosePlace} />;
      case 'ready':
        return <RouteSummary route={routeState.route} isRefreshing={false} />;
      case 'error':
        return <Text style={[styles.message, styles.error]}>{routeState.message}</Text>;
    }
  })();

  if (!content) return null;

  return (
    <View style={[styles.card, isWide ? styles.cardWide : styles.cardSheet]} onLayout={onLayout}>
      {!isWide && <View style={styles.grabber} />}
      <ScrollView bounces={false} showsVerticalScrollIndicator={false}>
        {content}
      </ScrollView>
    </View>
  );
}

const styles = StyleSheet.create({
  card: {
    backgroundColor: palette.panel,
    borderColor: palette.hairline,
    borderWidth: 1,
    padding: 18,
    shadowColor: '#000',
    shadowOpacity: 0.35,
    shadowRadius: 24,
    shadowOffset: { width: 0, height: 8 },
  },
  cardWide: { width: 360, maxHeight: '80%', borderRadius: radius.large, marginTop: 12, marginLeft: 16 },
  cardSheet: {
    position: 'absolute',
    left: 0,
    right: 0,
    bottom: 0,
    borderTopLeftRadius: radius.large,
    borderTopRightRadius: radius.large,
    borderBottomWidth: 0,
    paddingTop: 10,
    paddingBottom: 24,
    maxHeight: '45%',
  },
  grabber: {
    alignSelf: 'center',
    width: 36,
    height: 4,
    borderRadius: 2,
    backgroundColor: palette.hairline,
    marginBottom: 12,
  },
  refreshing: { opacity: 0.6 },
  summaryRow: { flexDirection: 'row', alignItems: 'baseline', justifyContent: 'space-between' },
  duration: { fontFamily, fontSize: 28, fontWeight: '600', color: palette.textPrimary, letterSpacing: -0.5 },
  arrival: { fontFamily, fontSize: 13, color: palette.textSecondary },
  subline: { fontFamily, fontSize: 12, color: palette.textSecondary, marginTop: 4 },
  entryway: { fontFamily, fontSize: 12, color: palette.accent, marginTop: 4 },
  legs: { marginTop: 14, gap: 10 },
  legRow: { flexDirection: 'row', alignItems: 'center', gap: 12 },
  legIcon: {
    width: 28,
    height: 28,
    borderRadius: 14,
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: palette.ink,
    borderWidth: 1,
    borderColor: palette.hairline,
  },
  legText: { flex: 1, minWidth: 0 },
  legTitle: { fontFamily, fontSize: 14, color: palette.textPrimary, fontWeight: '500' },
  legDetail: { fontFamily, fontSize: 12, color: palette.textSecondary, marginTop: 1 },
  legDuration: { fontFamily, fontSize: 13, color: palette.textSecondary },
  linkButton: { alignSelf: 'flex-start', marginTop: 4 },
  linkText: { fontFamily, fontSize: 12, color: palette.accent, fontWeight: '500' },
  prompt: { fontFamily, fontSize: 15, color: palette.textPrimary, fontWeight: '600' },
  choices: { marginTop: 8 },
  choice: { paddingVertical: 10, paddingHorizontal: 8, borderRadius: radius.small },
  choiceHovered: { backgroundColor: palette.raised },
  choiceName: { fontFamily, fontSize: 14, color: palette.textPrimary },
  choiceAddress: { fontFamily, fontSize: 12, color: palette.textSecondary, marginTop: 2 },
  message: { fontFamily, fontSize: 14, color: palette.textSecondary },
  messageStrong: { color: palette.textPrimary },
  error: { color: palette.danger },
  skeletonBar: { backgroundColor: palette.raised, borderRadius: 4 },
});
