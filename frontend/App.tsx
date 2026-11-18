import { StatusBar } from 'expo-status-bar';
import { useState } from 'react';
import { StyleSheet, useWindowDimensions, View } from 'react-native';
import { MapCanvas } from './src/map/MapCanvas';
import { useCurrentLocation } from './src/state/useCurrentLocation';
import { usePreferences } from './src/state/usePreferences';
import { useTrip } from './src/state/useTrip';
import { palette } from './src/theme/palette';
import { RouteCard } from './src/trip/RouteCard';
import { TopBar } from './src/trip/TopBar';

const wideLayoutMinWidth = 768;
const wideCardColumnWidth = 392;

export default function App() {
  const { width } = useWindowDimensions();
  const isWide = width >= wideLayoutMinWidth;
  const location = useCurrentLocation();
  const currentLocation = location.status === 'located' ? location.coordinate : null;
  const { preferences, toggleMode } = usePreferences();
  const trip = useTrip(currentLocation, preferences);
  const [topBarHeight, setTopBarHeight] = useState(0);
  const [cardHeight, setCardHeight] = useState(0);

  const route = trip.route;
  const hasCard = trip.routeState.status !== 'idle';

  return (
    <View style={styles.root}>
      <StatusBar style="light" />
      <MapCanvas
        route={route}
        currentLocation={currentLocation}
        pinnedOrigin={trip.pinnedOrigin}
        onMapPress={trip.isWaitingForLocation ? trip.pinOrigin : undefined}
        insets={{
          top: isWide ? 0 : topBarHeight,
          right: 0,
          bottom: !isWide && hasCard ? cardHeight : 0,
          left: isWide ? wideCardColumnWidth : 0,
        }}
      />
      <View style={styles.overlay} pointerEvents="box-none">
        <TopBar
          trip={trip}
          preferences={preferences}
          onToggleMode={toggleMode}
          isWide={isWide}
          onLayout={(event) => setTopBarHeight(event.nativeEvent.layout.height)}
        />
        <View style={styles.cardArea} pointerEvents="box-none">
          <RouteCard
            routeState={trip.routeState}
            isLocationUnavailable={location.status === 'unavailable'}
            isWide={isWide}
            isLive={trip.isLive}
            onChoosePlace={trip.choosePlace}
            onSelectOption={trip.selectOption}
            onStart={trip.start}
            onEnd={trip.end}
            onLayout={(event) => setCardHeight(event.nativeEvent.layout.height)}
          />
        </View>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1, backgroundColor: palette.mapBackground, overflow: 'hidden' },
  overlay: { ...StyleSheet.absoluteFill },
  cardArea: { flex: 1 },
});
