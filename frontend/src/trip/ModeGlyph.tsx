import Svg, { Path } from 'react-native-svg';
import { Mode } from '../api/types';
import { modeAppearance } from '../theme/modes';

export function ModeGlyph({ mode, size = 16, color }: { mode: Mode; size?: number; color?: string }) {
  const appearance = modeAppearance[mode];
  return (
    <Svg width={size} height={size} viewBox="2 2 20 20">
      <Path d={appearance.glyph} fill={color ?? appearance.color} />
    </Svg>
  );
}
