export function decodePolyline(encoded: string): [number, number][] {
  const positions: [number, number][] = [];
  let index = 0;
  let latitude = 0;
  let longitude = 0;

  function nextDelta() {
    let result = 0;
    let shift = 0;
    let byte: number;
    do {
      byte = encoded.charCodeAt(index++) - 63;
      result |= (byte & 0x1f) << shift;
      shift += 5;
    } while (byte >= 0x20);
    return result & 1 ? ~(result >> 1) : result >> 1;
  }

  while (index < encoded.length) {
    latitude += nextDelta();
    longitude += nextDelta();
    positions.push([longitude / 1e5, latitude / 1e5]);
  }
  return positions;
}
