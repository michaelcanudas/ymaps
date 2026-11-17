package bus

import (
	"sort"
	"time"

	"github.com/MobilityData/gtfs-realtime-bindings/golang/gtfs"
	"google.golang.org/protobuf/proto"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/plan"
)

type stopDelay struct {
	sequence  uint32
	arrival   int32
	departure int32
	skipped   bool
}

type tripDelay struct {
	date     int32
	canceled bool
	stops    []stopDelay
}

type tripUpdates struct {
	schedule *schedule
	at       time.Time
	trips    map[int32]tripDelay
}

type vehiclePositions struct {
	schedule *schedule
	at       time.Time
	byTrip   map[int32]plan.Vehicle
}

func feedTime(feed *gtfs.FeedMessage, fallback time.Time) time.Time {
	if stamp := feed.GetHeader().GetTimestamp(); stamp > 0 {
		return time.Unix(int64(stamp), 0)
	}
	return fallback
}

func parseTripUpdates(data []byte, s *schedule, now time.Time) (*tripUpdates, error) {
	feed := &gtfs.FeedMessage{}
	if err := proto.Unmarshal(data, feed); err != nil {
		return nil, err
	}
	updates := &tripUpdates{schedule: s, at: feedTime(feed, now), trips: map[int32]tripDelay{}}
	for _, entity := range feed.GetEntity() {
		update := entity.GetTripUpdate()
		if update == nil || entity.GetIsDeleted() {
			continue
		}
		number, ok := s.tripIndex[update.GetTrip().GetTripId()]
		if !ok {
			continue
		}
		t := s.trips[number]
		day, ok := serviceDayOfDate(update.GetTrip().GetStartDate())
		if !ok {
			day = s.likelyServiceDay(t, updates.at)
		}
		delay := tripDelay{date: day.date}
		if update.GetTrip().GetScheduleRelationship() == gtfs.TripDescriptor_CANCELED {
			delay.canceled = true
			updates.trips[number] = delay
			continue
		}
		searchFrom := t.first
		for _, stopUpdate := range update.GetStopTimeUpdate() {
			position, found := s.matchStopUpdate(t, stopUpdate, searchFrom)
			if !found {
				continue
			}
			searchFrom = position + 1
			scheduled := s.times[position]
			relationship := stopUpdate.GetScheduleRelationship()
			if relationship == gtfs.TripUpdate_StopTimeUpdate_NO_DATA {
				continue
			}
			arrival, arrivalKnown := eventDelay(stopUpdate.GetArrival(), day.at(scheduled.arrival))
			departure, departureKnown := eventDelay(stopUpdate.GetDeparture(), day.at(scheduled.departure))
			if !arrivalKnown && departureKnown {
				arrival = departure
			}
			if !departureKnown && arrivalKnown {
				departure = arrival
			}
			skipped := relationship == gtfs.TripUpdate_StopTimeUpdate_SKIPPED
			if !arrivalKnown && !departureKnown && !skipped {
				continue
			}
			if !arrivalKnown && !departureKnown && len(delay.stops) > 0 {
				arrival = delay.stops[len(delay.stops)-1].arrival
				departure = delay.stops[len(delay.stops)-1].departure
			}
			delay.stops = append(delay.stops, stopDelay{sequence: scheduled.sequence, arrival: arrival, departure: departure, skipped: skipped})
		}
		if len(delay.stops) == 0 {
			continue
		}
		sort.Slice(delay.stops, func(i, j int) bool { return delay.stops[i].sequence < delay.stops[j].sequence })
		updates.trips[number] = delay
	}
	return updates, nil
}

func (s *schedule) matchStopUpdate(t trip, update *gtfs.TripUpdate_StopTimeUpdate, searchFrom int32) (int32, bool) {
	if update.StopSequence != nil {
		return s.positionOfSequence(t, update.GetStopSequence())
	}
	stopID := update.GetStopId()
	if stopID == "" {
		return 0, false
	}
	for position := searchFrom; position < t.first+t.count; position++ {
		if s.stops[s.times[position].stop].id == stopID {
			return position, true
		}
	}
	return 0, false
}

func eventDelay(event *gtfs.TripUpdate_StopTimeEvent, scheduled time.Time) (int32, bool) {
	if event == nil {
		return 0, false
	}
	if event.Time != nil && event.GetTime() > 0 {
		return int32(event.GetTime() - scheduled.Unix()), true
	}
	if event.Delay != nil {
		return event.GetDelay(), true
	}
	return 0, false
}

func (s *schedule) likelyServiceDay(t trip, at time.Time) serviceDay {
	times := s.tripTimes(t)
	today := serviceDayOf(at, 0)
	if len(times) == 0 {
		return today
	}
	yesterday := serviceDayOf(at, -1)
	slack := time.Hour
	end := yesterday.at(times[len(times)-1].arrival).Add(slack)
	if at.Before(end) && times[len(times)-1].arrival >= 24*3600 {
		return yesterday
	}
	return today
}

func (d tripDelay) at(sequence uint32) (arrival, departure int32, usable bool) {
	if len(d.stops) == 0 {
		return 0, 0, true
	}
	if sequence < d.stops[0].sequence {
		return 0, 0, false
	}
	index := sort.Search(len(d.stops), func(i int) bool { return d.stops[i].sequence > sequence }) - 1
	found := d.stops[index]
	if found.sequence == sequence && found.skipped {
		return 0, 0, false
	}
	return found.arrival, found.departure, true
}

func parseVehiclePositions(data []byte, s *schedule, now time.Time) (*vehiclePositions, error) {
	feed := &gtfs.FeedMessage{}
	if err := proto.Unmarshal(data, feed); err != nil {
		return nil, err
	}
	positions := &vehiclePositions{schedule: s, at: feedTime(feed, now), byTrip: map[int32]plan.Vehicle{}}
	for _, entity := range feed.GetEntity() {
		vehicle := entity.GetVehicle()
		if vehicle == nil || vehicle.GetPosition() == nil || entity.GetIsDeleted() {
			continue
		}
		number, ok := s.tripIndex[vehicle.GetTrip().GetTripId()]
		if !ok {
			continue
		}
		if stamp := vehicle.GetTimestamp(); stamp > 0 && positions.at.Sub(time.Unix(int64(stamp), 0)) > vehicleFreshness {
			continue
		}
		id := vehicle.GetVehicle().GetId()
		if id == "" {
			id = entity.GetId()
		}
		found := plan.Vehicle{
			ID:   id,
			Name: vehicle.GetVehicle().GetLabel(),
			At:   geo.LatLon{Lat: float64(vehicle.GetPosition().GetLatitude()), Lon: float64(vehicle.GetPosition().GetLongitude())},
		}
		if vehicle.GetPosition().Bearing != nil {
			heading := int(vehicle.GetPosition().GetBearing()) % 360
			found.Heading = &heading
		}
		positions.byTrip[number] = found
	}
	return positions, nil
}
