package bus

import (
	"sort"

	"ymaps/backend/internal/geo"
)

func (s *schedule) linePath(t trip) []geo.LatLon {
	if t.shape >= 0 && len(s.shapes[t.shape].points) > 1 {
		return s.shapes[t.shape].points
	}
	times := s.tripTimes(t)
	path := make([]geo.LatLon, len(times))
	for index, time := range times {
		path[index] = s.stops[time.stop].at
	}
	return path
}

func (s *schedule) legPath(t trip, board, alight int32) []geo.LatLon {
	from := s.stops[s.times[board].stop].at
	to := s.stops[s.times[alight].stop].at
	if t.shape < 0 || len(s.shapes[t.shape].points) < 2 {
		path := make([]geo.LatLon, 0, alight-board+1)
		for position := board; position <= alight; position++ {
			path = append(path, s.stops[s.times[position].stop].at)
		}
		return path
	}
	line := s.shapes[t.shape]
	start := shapeIndex(line, s.times[board].dist, from, 0)
	end := shapeIndex(line, s.times[alight].dist, to, start)
	if end <= start {
		return []geo.LatLon{from, to}
	}
	path := make([]geo.LatLon, end-start+1)
	copy(path, line.points[start:end+1])
	return path
}

func shapeIndex(line shape, dist float32, at geo.LatLon, minimum int) int {
	if line.dist != nil && !isMissing(dist) {
		index := sort.Search(len(line.dist), func(i int) bool { return line.dist[i] >= dist })
		if index >= len(line.dist) {
			index = len(line.dist) - 1
		}
		if index > 0 && dist-line.dist[index-1] < line.dist[index]-dist {
			index--
		}
		if index < minimum {
			index = minimum
		}
		return index
	}
	best := minimum
	bestDistance := -1.0
	for index := minimum; index < len(line.points); index++ {
		distance := geo.Distance(line.points[index], at)
		if bestDistance < 0 || distance < bestDistance {
			best, bestDistance = index, distance
		}
	}
	return best
}
