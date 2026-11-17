package gbfs

import (
	"context"
	"strings"
	"time"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/plan"
	"ymaps/backend/internal/streets"
)

const veoDiscovery = "https://cluster-prod.veoride.com/api/shares/name/nhv/gbfs"

const (
	scooterSpeedKmh     = 15.0
	scooterLateSpeedKmh = 12.0
	eBikeSpeedKmh       = 17.0
	findVehicleP90      = 2 * time.Minute
	rangeSafetyFactor   = 1.3
	rangeReserveMeters  = 500.0
	sameSpotMeters      = 15.0
	companionMeters     = 30.0
	veoVehiclesPerEnd   = 5
)

type VeoMode struct {
	streets streets.Streets
	reader  *Reader
	now     func() time.Time
}

func NewVeo(s streets.Streets) *VeoMode {
	return newVeo(s, veoDiscovery)
}

func newVeo(s streets.Streets, discoveryURL string) *VeoMode {
	return &VeoMode{
		streets: s,
		now:     time.Now,
		reader: NewReader(Config{
			Name:         "veo",
			DiscoveryURL: discoveryURL,
			Feeds: []Feed{
				{Name: FreeBikeStatus, Required: true},
				{Name: VehicleTypes},
				{Name: SystemPricingPlans, Required: true},
				{Name: StationInformation},
				{Name: StationStatus},
			},
			Derive: DeriveSpots,
		}),
	}
}

func (m *VeoMode) Name() plan.ModeName {
	return plan.Veo
}

func (m *VeoMode) Run(ctx context.Context) {
	m.reader.Run(ctx)
}

type veoVehicle struct {
	vehicle    FreeVehicle
	form       string
	plan       PricingPlan
	speedKmh   float64
	companions int
}

func (v veoVehicle) point() plan.Point {
	return plan.Point{ID: "veo:" + v.vehicle.ID, Name: v.name(), At: v.vehicle.At}
}

func (v veoVehicle) name() string {
	if v.form == "bike" {
		return "Veo e-bike"
	}
	return "Veo scooter"
}

func (v veoVehicle) profile() streets.Profile {
	return streets.Profile{Kind: streets.Bicycle, SpeedKmh: v.speedKmh}
}

func (v veoVehicle) late(ride time.Duration) time.Duration {
	return scaleDuration(ride, scooterSpeedKmh/scooterLateSpeedKmh) + unlockAndParkP90 + findVehicleP90
}

func (v veoVehicle) reaches(meters float64) bool {
	if v.vehicle.RangeMeters == nil {
		return true
	}
	return *v.vehicle.RangeMeters >= rangeSafetyFactor*meters+rangeReserveMeters
}

func (m *VeoMode) Options(ctx context.Context, trip plan.Trip) ([]plan.Option, error) {
	snapshot := m.reader.Fresh(m.now())
	if snapshot == nil {
		return nil, nil
	}
	tripMeters := geo.Distance(trip.Origin.At, trip.Destination.At) * rangeSafetyFactor
	vehicles := nearestVeoVehicles(snapshot, trip.Origin.At, tripMeters)
	spots := nearestSpots(snapshot.Spots, trip.Destination.At)
	if len(vehicles) == 0 || len(spots) == 0 {
		return nil, nil
	}
	vehiclePoints := make([]geo.LatLon, len(vehicles))
	for i, vehicle := range vehicles {
		vehiclePoints[i] = vehicle.vehicle.At
	}
	spotPoints := make([]geo.LatLon, len(spots))
	for i, spot := range spots {
		spotPoints[i] = spot.At
	}
	walkIn, walkOut, err := walkTimes(ctx, m.streets, trip.Origin.At, vehiclePoints, spotPoints, trip.Destination.At)
	if err != nil {
		return nil, err
	}
	rides, err := m.rideMatrix(ctx, vehicles, spotPoints)
	if err != nil {
		return nil, err
	}
	pairs := bestPairs(len(vehicles), len(spots), func(i, j int) (time.Duration, bool) {
		meters := geo.Distance(vehiclePoints[i], spotPoints[j])
		if meters < companionMeters || !vehicles[i].reaches(meters*rangeSafetyFactor) {
			return 0, false
		}
		late := scaleDuration(walkIn[i], estimatedLateWalk) + vehicles[i].late(rides[i][j]) + loneVehicleRisk(walkIn[i], vehicles[i].companions) + scaleDuration(walkOut[j], estimatedLateWalk)
		return late, true
	})
	var options []plan.Option
	for _, chosen := range pairs {
		vehicle := vehicles[chosen.pickup]
		spot := spots[chosen.dropoff]
		trace, err := m.streets.Route(ctx, vehicle.vehicle.At, spot.At, vehicle.profile())
		if err != nil || !vehicle.reaches(trace.Meters) {
			continue
		}
		rideP50 := trace.Duration + unlockAndParkP50
		rideP90 := vehicle.late(trace.Duration) + loneVehicleRisk(walkIn[chosen.pickup], vehicle.companions)
		leg := plan.Leg{
			Mode:      plan.Veo,
			From:      vehicle.point(),
			To:        plan.Point{ID: "veo:spot:" + spot.ID, Name: "Veo parking", At: spot.At},
			CostCents: vehicle.plan.Cents(rideP50.Minutes()),
			Path:      trace.Path,
			Vehicle:   &plan.Vehicle{ID: vehicle.vehicle.ID, Name: vehicle.name(), At: vehicle.vehicle.At},
			Form:      vehicle.form,
			RentalURI: vehicle.vehicle.RentalURI,
		}
		option, err := assemble(ctx, m.streets, trip, leg, rideP50, rideP90)
		if err != nil {
			continue
		}
		options = append(options, option)
	}
	return options, nil
}

func (m *VeoMode) rideMatrix(ctx context.Context, vehicles []veoVehicle, spots []geo.LatLon) ([][]time.Duration, error) {
	rides := make([][]time.Duration, len(vehicles))
	groups := map[float64][]int{}
	for i, vehicle := range vehicles {
		groups[vehicle.speedKmh] = append(groups[vehicle.speedKmh], i)
	}
	for _, members := range groups {
		from := make([]geo.LatLon, len(members))
		for k, index := range members {
			from[k] = vehicles[index].vehicle.At
		}
		durations, err := m.streets.Matrix(ctx, from, spots, vehicles[members[0]].profile())
		if err != nil {
			return nil, err
		}
		for k, index := range members {
			rides[index] = durations[k]
		}
	}
	return rides, nil
}

func usableVeoVehicle(snapshot *Snapshot, vehicle FreeVehicle) (veoVehicle, bool) {
	pricing, priced := snapshot.Plans[vehicle.PlanID]
	if !priced {
		return veoVehicle{}, false
	}
	usable := veoVehicle{vehicle: vehicle, form: "scooter", plan: pricing, speedKmh: scooterSpeedKmh}
	kind, known := snapshot.Types[vehicle.TypeID]
	if known && kind.Form == "bicycle" {
		usable.form = "bike"
		if strings.HasPrefix(kind.Propulsion, "electric") {
			usable.speedKmh = eBikeSpeedKmh
		}
	}
	return usable, true
}

func nearestVeoVehicles(snapshot *Snapshot, origin geo.LatLon, tripMeters float64) []veoVehicle {
	var usable []veoVehicle
	for _, vehicle := range snapshot.Vehicles {
		candidate, ok := usableVeoVehicle(snapshot, vehicle)
		if ok && candidate.reaches(tripMeters) {
			usable = append(usable, candidate)
		}
	}
	points := make([]geo.LatLon, len(usable))
	for i, candidate := range usable {
		points[i] = candidate.vehicle.At
	}
	var chosen []veoVehicle
	for _, index := range nearestIndices(points, origin, len(points)) {
		candidate := usable[index]
		duplicate := false
		for _, existing := range chosen {
			if geo.Distance(existing.vehicle.At, candidate.vehicle.At) < sameSpotMeters {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		for _, other := range usable {
			if geo.Distance(other.vehicle.At, candidate.vehicle.At) <= companionMeters {
				candidate.companions++
			}
		}
		chosen = append(chosen, candidate)
		if len(chosen) == veoVehiclesPerEnd {
			break
		}
	}
	return chosen
}

func nearestSpots(spots []Spot, destination geo.LatLon) []Spot {
	points := make([]geo.LatLon, len(spots))
	for i, spot := range spots {
		points[i] = spot.At
	}
	var chosen []Spot
	for _, index := range nearestIndices(points, destination, candidatesPerEnd) {
		chosen = append(chosen, spots[index])
	}
	return chosen
}
