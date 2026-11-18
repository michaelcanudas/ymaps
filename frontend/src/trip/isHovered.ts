import { PressableStateCallbackType } from 'react-native';

export function isHovered(state: PressableStateCallbackType) {
  return Boolean((state as PressableStateCallbackType & { hovered?: boolean }).hovered);
}
