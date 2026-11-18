import { StyleSheet, Text, View } from 'react-native';
import { Leg } from '../api/types';
import { modeAppearance } from '../theme/modes';
import { fontFamily, palette } from '../theme/palette';
import { legSeconds } from './legTiming';
import { ModeGlyph } from './ModeGlyph';

const shortWalkSeconds = 45;

export function shortLineName(name: string) {
  const first = name.split(' ')[0];
  if (first && /^\d+$/.test(first)) return first;
  return name.split('-')[0].trim();
}

function LegChip({ leg }: { leg: Leg }) {
  const isRide = leg.mode !== 'walk';
  const color = modeAppearance[leg.mode].color;
  const minutes = Math.max(1, Math.round(legSeconds(leg) / 60));
  const line = leg.mode === 'shuttle' || leg.mode === 'bus' ? leg.line : null;
  return (
    <View
      style={[styles.chip, isRide ? { backgroundColor: `${color}22`, borderColor: `${color}66` } : styles.walkChip]}
    >
      <ModeGlyph mode={leg.mode} size={isRide ? 20 : 14} />
      {line ? (
        <Text style={[styles.chipText, { color }]} numberOfLines={1}>
          {shortLineName(line.name)}
        </Text>
      ) : null}
      <Text style={[styles.chipText, isRide ? { color } : styles.walkText, line && styles.secondary]}>{minutes}m</Text>
    </View>
  );
}

export function LegStrip({ legs }: { legs: Leg[] }) {
  const shown = legs.filter((leg) => leg.mode !== 'walk' || legSeconds(leg) >= shortWalkSeconds);
  return (
    <View style={styles.strip}>
      {shown.map((leg, index) => (
        <View key={index} style={styles.step}>
          {index > 0 ? <Text style={styles.chevron}>›</Text> : null}
          <LegChip leg={leg} />
        </View>
      ))}
    </View>
  );
}

const styles = StyleSheet.create({
  strip: { flexDirection: 'row', flexWrap: 'wrap', alignItems: 'center', rowGap: 6, marginTop: 14 },
  step: { flexDirection: 'row', alignItems: 'center' },
  chevron: { fontFamily, fontSize: 16, color: palette.textFaint, marginHorizontal: 6 },
  chip: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 5,
    height: 30,
    paddingHorizontal: 9,
    borderRadius: 15,
    borderWidth: 1,
    borderColor: 'transparent',
  },
  walkChip: { paddingHorizontal: 2 },
  chipText: { fontFamily, fontSize: 13, fontWeight: '600' },
  walkText: { color: palette.textSecondary },
  secondary: { opacity: 0.75 },
});
