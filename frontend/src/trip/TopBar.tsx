import { LayoutChangeEvent, Pressable, StyleSheet, Text, TextInput, View } from 'react-native';
import { AvoidableMode } from '../api/types';
import { Preferences } from '../state/usePreferences';
import { Trip } from '../state/useTrip';
import { fontFamily, palette, radius } from '../theme/palette';
import { ModePicker } from './ModePicker';

type TopBarProps = {
  trip: Trip;
  preferences: Preferences;
  onToggleMode: (mode: AvoidableMode) => void;
  isWide: boolean;
  onLayout: (event: LayoutChangeEvent) => void;
};

export function TopBar({ trip, preferences, onToggleMode, isWide, onLayout }: TopBarProps) {
  return (
    <View style={[styles.bar, isWide && styles.floating]} onLayout={onLayout}>
      <View style={styles.field}>
        <View style={styles.marker} />
        <TextInput
          value={trip.destination}
          placeholder="Where to?"
          placeholderTextColor={palette.textSecondary}
          autoFocus
          autoCorrect={false}
          autoCapitalize="none"
          returnKeyType="go"
          onChangeText={trip.editDestination}
          onSubmitEditing={trip.submit}
          style={styles.input}
        />
        {trip.destination ? (
          <Pressable accessibilityLabel="Clear" onPress={trip.clearTrip} style={styles.clear}>
            <Text style={styles.clearText}>×</Text>
          </Pressable>
        ) : null}
      </View>
      <ModePicker preferences={preferences} onToggleMode={onToggleMode} />
    </View>
  );
}

const styles = StyleSheet.create({
  bar: {
    backgroundColor: palette.ink,
    paddingHorizontal: 16,
    paddingVertical: 12,
    borderBottomWidth: 1,
    borderBottomColor: palette.hairline,
  },
  floating: {
    width: 360,
    marginTop: 16,
    marginLeft: 16,
    padding: 12,
    borderRadius: radius.large,
    borderWidth: 1,
    borderColor: palette.hairline,
    shadowColor: '#000',
    shadowOpacity: 0.35,
    shadowRadius: 24,
    shadowOffset: { width: 0, height: 8 },
  },
  field: {
    flexDirection: 'row',
    alignItems: 'center',
    height: 44,
    paddingLeft: 14,
    paddingRight: 6,
    borderRadius: radius.medium,
    backgroundColor: palette.raised,
  },
  marker: { width: 8, height: 8, borderRadius: 1, backgroundColor: palette.textPrimary, marginRight: 12 },
  input: {
    flex: 1,
    minWidth: 0,
    height: '100%',
    fontFamily,
    fontSize: 16,
    color: palette.textPrimary,
    outlineStyle: 'none',
  } as object,
  clear: { width: 32, height: 32, alignItems: 'center', justifyContent: 'center' },
  clearText: { color: palette.textSecondary, fontSize: 18, lineHeight: 20 },
});
