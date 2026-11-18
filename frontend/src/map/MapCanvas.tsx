import { StyleSheet, View } from 'react-native';
import { palette } from '../theme/palette';
import { MapCanvasProps } from './mapProps';

export function MapCanvas(_props: MapCanvasProps) {
  return <View style={[StyleSheet.absoluteFill, { backgroundColor: palette.mapBackground }]} />;
}
