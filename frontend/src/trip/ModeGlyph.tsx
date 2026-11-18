import { Image, ImageSourcePropType } from 'react-native';
import Svg, { Path } from 'react-native-svg';
import { Mode } from '../api/types';
import { modeAppearance } from '../theme/modes';
import { palette } from '../theme/palette';

const vehicleIcons: Partial<Record<Mode, ImageSourcePropType>> = {
  bus: require('../../assets/modes/bus.png'),
  shuttle: require('../../assets/modes/shuttle.png'),
  bike: require('../../assets/modes/bike.png'),
  veo: require('../../assets/modes/veo.png'),
};

export function ModeGlyph({ mode, size = 16, muted = false }: { mode: Mode; size?: number; muted?: boolean }) {
  const icon = vehicleIcons[mode];
  if (icon) {
    return <Image source={icon} style={{ width: size, height: size, opacity: muted ? 0.35 : 1 }} />;
  }
  const appearance = modeAppearance[mode];
  return (
    <Svg width={size} height={size} viewBox="2 2 20 20">
      <Path d={appearance.glyph} fill={muted ? palette.textFaint : appearance.color} />
    </Svg>
  );
}
