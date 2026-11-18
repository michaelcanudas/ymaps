import { StyleSheet, View } from 'react-native';
import { Leg } from '../api/types';
import { modeAppearance } from '../theme/modes';
import { legSeconds } from './legTiming';

export function LegStrip({ legs }: { legs: Leg[] }) {
  return (
    <View style={styles.strip}>
      {legs.map((leg, index) => (
        <View
          key={index}
          style={[
            styles.segment,
            {
              flexGrow: Math.max(legSeconds(leg), 30),
              backgroundColor: modeAppearance[leg.mode].color,
              opacity: leg.mode === 'walk' ? 0.45 : 0.9,
            },
          ]}
        />
      ))}
    </View>
  );
}

const styles = StyleSheet.create({
  strip: { flexDirection: 'row', gap: 3, height: 6, marginTop: 12 },
  segment: { flexBasis: 0, borderRadius: 3 },
});
