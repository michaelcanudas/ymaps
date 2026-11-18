package gbfs

import (
	"context"
	"strings"
	"time"

	"ymaps/backend/internal/plan"
	"ymaps/backend/internal/streets"
)

const veoDiscovery = "https://cluster-prod.veoride.com/api/shares/name/nhv/gbfs"

const (
	scooterSpeedKmh = 13.0
	eBikeSpeedKmh   = 15.0
)

type SpotSource interface {
	Spots() []Spot
}

type VeoMode struct {
	reader *Reader
	spots  SpotSource
	now    func() time.Time
}

func NewVeo(spots SpotSource) *VeoMode {
	return newVeo(veoDiscovery, spots)
}

func newVeo(discoveryURL string, spots SpotSource) *VeoMode {
	return &VeoMode{
		now:   time.Now,
		spots: spots,
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

func (m *VeoMode) Reader() *Reader {
	return m.reader
}

func (m *VeoMode) Name() plan.ModeName {
	return plan.Veo
}

func (m *VeoMode) Run(ctx context.Context) {
	m.reader.Run(ctx)
}

func (m *VeoMode) Network(_ context.Context, query plan.Query) (plan.Network, error) {
	snapshot := m.reader.Fresh(m.now())
	if snapshot == nil {
		return plan.Network{}, nil
	}
	spots := snapshot.Spots
	if m.spots != nil {
		if learned := m.spots.Spots(); len(learned) > 0 {
			spots = learned
		}
	}
	rental := plan.Rental{Mode: plan.Veo}
	for _, spot := range spots {
		if query.Covers(spot.At) {
			rental.Dropoffs = append(rental.Dropoffs, plan.Point{ID: "veo:spot:" + spot.ID, Name: "Veo parking", At: spot.At})
		}
	}
	for _, vehicle := range snapshot.Vehicles {
		if !query.Covers(vehicle.At) {
			continue
		}
		if pickup, ok := veoPickup(snapshot, vehicle); ok {
			rental.Pickups = append(rental.Pickups, pickup)
		}
	}
	if len(rental.Pickups) == 0 || len(rental.Dropoffs) == 0 {
		return plan.Network{}, nil
	}
	return plan.Network{Rentals: []plan.Rental{rental}}, nil
}

func veoPickup(snapshot *Snapshot, vehicle FreeVehicle) (plan.Pickup, bool) {
	pricing, priced := snapshot.Plans[vehicle.PlanID]
	if !priced {
		return plan.Pickup{}, false
	}
	form, name, speed := "scooter", "Veo scooter", scooterSpeedKmh
	if kind, known := snapshot.Types[vehicle.TypeID]; known && kind.Form == "bicycle" {
		form, name = "bike", "Veo e-bike"
		if strings.HasPrefix(kind.Propulsion, "electric") {
			speed = eBikeSpeedKmh
		}
	}
	point := plan.Point{ID: "veo:" + vehicle.ID, Name: name, At: vehicle.At}
	return plan.Pickup{
		Point:       point,
		Vehicle:     &plan.Vehicle{ID: vehicle.ID, Name: name, At: vehicle.At},
		Form:        form,
		RentalURI:   vehicle.RentalURI,
		Profile:     streets.Profile{Kind: streets.Bicycle, SpeedKmh: speed},
		RangeMeters: vehicle.RangeMeters,
		Unlock:      veoUnlockAndPark,
		Cost:        func(ride time.Duration) int { return pricing.Cents(ride.Minutes()) },
	}, true
}
