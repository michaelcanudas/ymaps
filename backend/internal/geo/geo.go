package geo

import (
	"math"
	"strings"
)

type LatLon struct {
	Lat float64
	Lon float64
}

const earthRadiusMeters = 6371008.8

func Distance(a, b LatLon) float64 {
	lat1 := radians(a.Lat)
	lat2 := radians(b.Lat)
	deltaLat := lat2 - lat1
	deltaLon := radians(b.Lon - a.Lon)
	h := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(deltaLon/2)*math.Sin(deltaLon/2)
	return 2 * earthRadiusMeters * math.Asin(math.Min(1, math.Sqrt(h)))
}

func PathLength(path []LatLon) float64 {
	total := 0.0
	for i := 1; i < len(path); i++ {
		total += Distance(path[i-1], path[i])
	}
	return total
}

func Encode(path []LatLon) string {
	var builder strings.Builder
	previousLat, previousLon := 0, 0
	for _, point := range path {
		lat := int(math.Round(point.Lat * 1e5))
		lon := int(math.Round(point.Lon * 1e5))
		writeSigned(&builder, lat-previousLat)
		writeSigned(&builder, lon-previousLon)
		previousLat, previousLon = lat, lon
	}
	return builder.String()
}

func Decode(encoded string) []LatLon {
	var path []LatLon
	lat, lon := 0, 0
	for index := 0; index < len(encoded); {
		var deltaLat, deltaLon int
		deltaLat, index = readSigned(encoded, index)
		deltaLon, index = readSigned(encoded, index)
		lat += deltaLat
		lon += deltaLon
		path = append(path, LatLon{Lat: float64(lat) / 1e5, Lon: float64(lon) / 1e5})
	}
	return path
}

func writeSigned(builder *strings.Builder, value int) {
	shifted := value << 1
	if value < 0 {
		shifted = ^shifted
	}
	for shifted >= 0x20 {
		builder.WriteByte(byte((0x20 | (shifted & 0x1f)) + 63))
		shifted >>= 5
	}
	builder.WriteByte(byte(shifted + 63))
}

func readSigned(encoded string, index int) (int, int) {
	result, shift := 0, 0
	for index < len(encoded) {
		chunk := int(encoded[index]) - 63
		index++
		result |= (chunk & 0x1f) << shift
		shift += 5
		if chunk < 0x20 {
			break
		}
	}
	if result&1 != 0 {
		return ^(result >> 1), index
	}
	return result >> 1, index
}

func radians(degrees float64) float64 {
	return degrees * math.Pi / 180
}
