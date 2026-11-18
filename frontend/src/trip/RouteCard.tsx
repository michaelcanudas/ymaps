import { useEffect, useState } from 'react';
import { LayoutChangeEvent, Linking, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import Svg, { Path } from 'react-native-svg';
import { AlternativeKind, Leg, Place, Route } from '../api/types';
import { RouteState } from '../state/useTrip';
import { modeAppearance } from '../theme/modes';
import { fontFamily, palette, radius } from '../theme/palette';
import { isHovered } from './isHovered';
import { clockLabel, costLabel, legSeconds, minutesLabel } from './legTiming';
import { LegStrip } from './LegStrip';
import { ModeGlyph } from './ModeGlyph';

type RouteCardProps = {
  routeState: RouteState;
  isLocationUnavailable: boolean;
  isWide: boolean;
  isLive: boolean;
  onChoosePlace: (place: Place) => void;
  onSelectOption: (index: number) => void;
  onStart: () => void;
  onEnd: () => void;
  onLayout: (event: LayoutChangeEvent) => void;
};

type TripActions = Pick<RouteCardProps, 'isLive' | 'onSelectOption' | 'onStart' | 'onEnd'>;

const kindIcons: Record<AlternativeKind | 'recommended' | 'other', string> = {
  recommended: 'M12 3.5l2.6 5.3 5.9.9-4.3 4.1 1 5.8-5.2-2.7-5.2 2.7 1-5.8-4.3-4.1 5.9-.9z',
  faster: 'M13.5 2.5 5 13.5h6l-1.5 8 8.5-11h-6z',
  free: 'M3.5 12.6V4.5a1 1 0 0 1 1-1h8.1l8 8-9.1 9.1zM8 9.6a1.6 1.6 0 1 0 0-3.2 1.6 1.6 0 0 0 0 3.2z',
  cheaper:
    'M11 3h2v2.1c2 .3 3.4 1.6 3.5 3.4h-2c-.1-1-1-1.7-2.5-1.7s-2.4.6-2.4 1.6c0 .9.7 1.3 2.6 1.8 2.5.6 4.4 1.4 4.4 3.8 0 1.9-1.4 3.1-3.6 3.4V19h-2v-1.6c-2.2-.3-3.7-1.7-3.8-3.7h2c.1 1.1 1.1 1.9 2.8 1.9 1.6 0 2.6-.7 2.6-1.7 0-1-.8-1.4-2.8-1.9-2.4-.6-4.2-1.4-4.2-3.7 0-1.7 1.3-2.9 3.4-3.2z',
  lessWalking: modeAppearance.walk.glyph,
  other: 'M7 4h2v6.2l6-3.4V5h4v4h-1.8l-8.2 4.6V20H7z',
};

function useNow(intervalMs: number) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(timer);
  }, [intervalMs]);
  return now;
}

function rideInstruction(leg: Leg) {
  const toName = leg.to.name ?? 'your destination';
  switch (leg.mode) {
    case 'walk':
      return `Walk to ${toName}`;
    case 'shuttle':
    case 'bus':
      return `${leg.line.name} at ${clockLabel(leg.departAt)}`;
    case 'bike':
      return `Bike to ${toName}`;
    case 'veo':
      return `Veo to ${toName}`;
  }
}

function KindIcon({ kind }: { kind: keyof typeof kindIcons }) {
  return (
    <Svg width={14} height={14} viewBox="2 2 20 20">
      <Path d={kindIcons[kind]} fill={kind === 'recommended' ? palette.textPrimary : palette.textSecondary} />
    </Svg>
  );
}

function OptionRow({ option, isMain, onPress }: { option: Route; isMain: boolean; onPress: () => void }) {
  const rides = option.legs.filter((leg) => leg.mode !== 'walk');
  return (
    <Pressable onPress={onPress} style={(state) => [styles.option, isHovered(state) && styles.optionHovered]}>
      <KindIcon kind={isMain ? 'recommended' : option.kind ?? 'other'} />
      <Text style={styles.optionLabel} numberOfLines={1}>
        {isMain ? 'Recommended' : option.label ?? 'Another way'}
      </Text>
      <View style={styles.optionModes}>
        {rides.map((leg, index) => (
          <ModeGlyph key={index} mode={leg.mode} size={18} />
        ))}
      </View>
      <Text style={styles.optionTime}>{clockLabel(option.arriveAt)}</Text>
    </Pressable>
  );
}

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
        <ModeGlyph mode={leg.mode} size={leg.mode === 'walk' ? 14 : 24} />
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

function RouteSummary({
  route,
  options,
  selected,
  isRefreshing,
  actions,
}: {
  route: Route;
  options: Route[];
  selected: number;
  isRefreshing: boolean;
  actions?: TripActions;
}) {
  const now = useNow(30000);
  const minutesAway = Math.max(1, Math.round((Date.parse(route.arriveAt) - now) / 60000));
  const firstRide = route.legs.find((leg) => leg.mode !== 'walk');
  const subtitle = [
    `Arrive ${clockLabel(route.arriveAt)}`,
    ...(firstRide ? [rideInstruction(firstRide)] : []),
    ...(route.costCents > 0 ? [costLabel(route.costCents)] : []),
  ].join(' · ');
  const showOptions = actions !== undefined && !actions.isLive && options.length > 1;
  return (
    <View style={isRefreshing && styles.refreshing}>
      <View style={styles.summaryRow}>
        <Text style={styles.minutes}>{minutesAway}</Text>
        <Text style={styles.minutesUnit}> min</Text>
        {actions?.isLive ? <Text style={styles.liveBadge}>Live</Text> : null}
      </View>
      <Text style={styles.destination} numberOfLines={1}>
        {route.place.entryway?.name ?? route.place.name}
      </Text>
      <Text style={styles.subline} numberOfLines={2}>
        {subtitle}
      </Text>
      <LegStrip legs={route.legs} />
      {showOptions ? (
        <View style={styles.options}>
          {options.map((option, index) =>
            index === selected ? null : (
              <OptionRow
                key={option.id}
                option={option}
                isMain={index === 0}
                onPress={() => actions.onSelectOption(index)}
              />
            ),
          )}
        </View>
      ) : null}
      <View style={styles.legs}>
        {route.legs.map((leg, index) => (
          <LegRow key={index} leg={leg} />
        ))}
      </View>
      {actions ? (
        <Pressable
          onPress={actions.isLive ? actions.onEnd : actions.onStart}
          style={(state) => [
            styles.tripButton,
            actions.isLive ? styles.endButton : styles.startButton,
            isHovered(state) && styles.tripButtonHovered,
          ]}
        >
          <Text style={[styles.tripButtonText, actions.isLive ? styles.endText : styles.startText]}>
            {actions.isLive ? 'End' : 'Start'}
          </Text>
        </Pressable>
      ) : null}
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

export function RouteCard({
  routeState,
  isLocationUnavailable,
  isWide,
  isLive,
  onChoosePlace,
  onSelectOption,
  onStart,
  onEnd,
  onLayout,
}: RouteCardProps) {
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
        return routeState.previous ? (
          <RouteSummary route={routeState.previous} options={[]} selected={0} isRefreshing />
        ) : (
          <Skeleton />
        );
      case 'choosing':
        return <PlaceChoices places={routeState.places} onChoose={onChoosePlace} />;
      case 'ready':
        return (
          <RouteSummary
            route={routeState.options[routeState.selected]}
            options={routeState.options}
            selected={routeState.selected}
            isRefreshing={false}
            actions={{ isLive, onSelectOption, onStart, onEnd }}
          />
        );
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
  cardWide: { width: 360, maxHeight: '80%', borderRadius: radius.large, marginTop: 8, marginLeft: 16 },
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
  summaryRow: { flexDirection: 'row', alignItems: 'baseline' },
  minutes: { fontFamily, fontSize: 40, fontWeight: '700', color: palette.textPrimary, letterSpacing: -1 },
  minutesUnit: { fontFamily, fontSize: 18, fontWeight: '600', color: palette.textPrimary },
  liveBadge: {
    marginLeft: 'auto',
    alignSelf: 'center',
    fontFamily,
    fontSize: 11,
    fontWeight: '600',
    letterSpacing: 0.5,
    textTransform: 'uppercase',
    color: palette.accent,
  },
  destination: { fontFamily, fontSize: 15, fontWeight: '600', color: palette.textPrimary, marginTop: 2 },
  subline: { fontFamily, fontSize: 12, color: palette.textSecondary, marginTop: 3 },
  options: { marginTop: 14 },
  option: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 10,
    paddingVertical: 10,
    paddingHorizontal: 4,
    borderTopWidth: 1,
    borderTopColor: palette.hairline,
  },
  optionHovered: { backgroundColor: palette.raised },
  optionLabel: { flex: 1, minWidth: 0, fontFamily, fontSize: 13, fontWeight: '500', color: palette.textPrimary },
  optionModes: { flexDirection: 'row', gap: 2 },
  optionTime: { fontFamily, fontSize: 13, color: palette.textSecondary, fontVariant: ['tabular-nums'] },
  tripButton: { marginTop: 16, height: 46, borderRadius: 23, alignItems: 'center', justifyContent: 'center' },
  startButton: { backgroundColor: palette.textPrimary },
  endButton: { backgroundColor: palette.raised, borderWidth: 1, borderColor: palette.hairline },
  tripButtonHovered: { opacity: 0.9 },
  tripButtonText: { fontFamily, fontSize: 15, fontWeight: '600' },
  startText: { color: palette.ink },
  endText: { color: palette.textPrimary },
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
  skeletonBar: { backgroundColor: palette.raised, borderRadius: radius.small },
});
