package gbfs

import (
	"fmt"
	"sort"

	"ymaps/backend/internal/geo"
)

const (
	spotLinkMeters  = 30.0
	spotMinVehicles = 3
	spotMaxVehicles = 30
)

type Spot struct {
	ID       string
	At       geo.LatLon
	Vehicles int
}

func DeriveSpots(snapshot *Snapshot) {
	snapshot.Spots = clusterSpots(snapshot.Vehicles)
}

func clusterSpots(vehicles []FreeVehicle) []Spot {
	parent := make([]int, len(vehicles))
	for i := range parent {
		parent[i] = i
	}
	var root func(int) int
	root = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	for i := range vehicles {
		for j := i + 1; j < len(vehicles); j++ {
			if geo.Distance(vehicles[i].At, vehicles[j].At) <= spotLinkMeters {
				parent[root(i)] = root(j)
			}
		}
	}
	members := map[int][]geo.LatLon{}
	for i, vehicle := range vehicles {
		group := root(i)
		members[group] = append(members[group], vehicle.At)
	}
	var spots []Spot
	for _, points := range members {
		if len(points) < spotMinVehicles || len(points) > spotMaxVehicles {
			continue
		}
		centroid := geo.LatLon{}
		for _, point := range points {
			centroid.Lat += point.Lat
			centroid.Lon += point.Lon
		}
		centroid.Lat /= float64(len(points))
		centroid.Lon /= float64(len(points))
		spots = append(spots, Spot{
			ID:       fmt.Sprintf("%.4f,%.4f", centroid.Lat, centroid.Lon),
			At:       centroid,
			Vehicles: len(points),
		})
	}
	sort.Slice(spots, func(i, j int) bool { return spots[i].ID < spots[j].ID })
	return spots
}
