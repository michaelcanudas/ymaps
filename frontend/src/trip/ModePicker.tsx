import { Pressable, ScrollView, StyleSheet } from 'react-native';
import { AvoidableMode, avoidableModes } from '../api/types';
import { Preferences } from '../state/usePreferences';
import { modeAppearance } from '../theme/modes';
import { palette, radius } from '../theme/palette';
import { isHovered } from './isHovered';
import { ModeGlyph } from './ModeGlyph';

type ModePickerProps = {
  preferences: Preferences;
  onToggleMode: (mode: AvoidableMode) => void;
};

export function ModePicker({ preferences, onToggleMode }: ModePickerProps) {
  return (
    <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={styles.row}>
      {avoidableModes.map((mode) => {
        const isOn = preferences.modes.includes(mode);
        const appearance = modeAppearance[mode];
        return (
          <Pressable
            key={mode}
            onPress={() => onToggleMode(mode)}
            accessibilityRole="switch"
            accessibilityState={{ checked: isOn }}
            accessibilityLabel={appearance.label}
            style={(state) => [styles.chip, isOn ? styles.chipOn : styles.chipOff, isHovered(state) && styles.chipHovered]}
          >
            <ModeGlyph mode={mode} size={24} muted={!isOn} />
          </Pressable>
        );
      })}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  row: { gap: 6, paddingTop: 10, alignItems: 'center' },
  chip: {
    height: 30,
    minWidth: 36,
    paddingHorizontal: 9,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: radius.capsule,
    borderWidth: 1,
  },
  chipOn: { backgroundColor: palette.raised, borderColor: palette.hairline },
  chipOff: { backgroundColor: 'transparent', borderColor: palette.hairline, opacity: 0.7 },
  chipHovered: { borderColor: palette.textFaint },
});
