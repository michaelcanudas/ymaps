package bus

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MobilityData/gtfs-realtime-bindings/golang/gtfs"
	"google.golang.org/protobuf/proto"

	"ymaps/backend/internal/geo"
	"ymaps/backend/internal/plan"
	"ymaps/backend/internal/streets"
)

var (
	origin      = plan.Point{Name: "Old Campus", At: geo.LatLon{Lat: 41.3002, Lon: -72.9301}}
	destination = plan.Point{Name: "Sachem St", At: geo.LatLon{Lat: 41.3148, Lon: -72.9301}}
)

func at(value string) time.Time {
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", value, newYork)
	if err != nil {
		panic(err)
	}
	return parsed
}

func loadedMode(t *testing.T, now time.Time) *Mode {
	t.Helper()
	s, err := parseSchedule(os.DirFS("testdata/feed"))
	if err != nil {
		t.Fatal(err)
	}
	m := New(streets.Estimate{})
	m.now = func() time.Time { return now }
	m.schedule.Store(s)
	return m
}

func options(t *testing.T, m *Mode, departAt time.Time) []plan.Option {
	t.Helper()
	found, err := m.Options(context.Background(), plan.Trip{Origin: origin, Destination: destination, DepartAt: departAt})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func TestParseKeepsOnlyNewHaven(t *testing.T) {
	s, err := parseSchedule(os.DirFS("testdata/feed"))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.routes) != 2 || len(s.trips) != 3 {
		t.Fatalf("routes %d trips %d", len(s.routes), len(s.trips))
	}
	if _, ok := s.tripIndex["T1"]; !ok {
		t.Fatal("byte order mark broke trip_id")
	}
	if _, ok := s.tripIndex["H1"]; ok {
		t.Fatal("kept a Hartford trip")
	}
	if s.routes[0].color != defaultColor {
		t.Fatalf("color %q", s.routes[0].color)
	}
	t1 := s.trips[s.tripIndex["T1"]]
	if third := s.tripTimes(t1)[2]; third.arrival != 8*3600+6*60 {
		t.Fatalf("interpolated arrival %d", third.arrival)
	}
	t2 := s.trips[s.tripIndex["T2"]]
	if times := s.tripTimes(t2); times[0].sequence != 1 || times[3].sequence != 4 {
		t.Fatal("stop times not ordered by sequence")
	}
	for _, found := range s.stops {
		if found.id == "H" || found.id == "X" {
			t.Fatalf("kept stop %s", found.id)
		}
	}
}

func TestOptionsBuildWalkBusWalk(t *testing.T) {
	departAt := at("2026-11-17 07:55:00")
	m := loadedMode(t, departAt)
	found := options(t, m, departAt)
	if len(found) != 2 {
		t.Fatalf("options %d", len(found))
	}
	first := found[0]
	if len(first.Legs) != 3 || first.Legs[0].Mode != plan.Walk || first.Legs[1].Mode != plan.Bus || first.Legs[2].Mode != plan.Walk {
		t.Fatalf("legs %+v", first.Legs)
	}
	walkTo, bus, walkFrom := first.Legs[0], first.Legs[1], first.Legs[2]
	if !bus.DepartAt.Equal(at("2026-11-17 08:00:00")) || !bus.ArriveAt.Equal(at("2026-11-17 08:09:00")) {
		t.Fatalf("bus %v %v", bus.DepartAt, bus.ArriveAt)
	}
	if !bus.ArriveAtLate.Equal(bus.ArriveAt.Add(lateMarginSchedule)) {
		t.Fatalf("late %v", bus.ArriveAtLate)
	}
	if walkTo.DepartAt.Before(departAt) || walkTo.ArriveAt.After(bus.DepartAt) {
		t.Fatalf("walk to stop %v %v", walkTo.DepartAt, walkTo.ArriveAt)
	}
	if bus.DepartAt.Sub(walkTo.ArriveAt) != boardingBuffer {
		t.Fatalf("buffer %v", bus.DepartAt.Sub(walkTo.ArriveAt))
	}
	if !walkFrom.DepartAt.Equal(bus.ArriveAt) || walkFrom.ArriveAtLate.Before(bus.ArriveAtLate) {
		t.Fatalf("walk from stop %v %v", walkFrom.DepartAt, walkFrom.ArriveAtLate)
	}
	if bus.From.ID != "bus:A" || bus.To.ID != "bus:D" || bus.From.Name != "Chapel St @ College St" {
		t.Fatalf("points %+v %+v", bus.From, bus.To)
	}
	if bus.Line.ID != "16228" || bus.Line.Name != "228 Whitney Ave" || bus.Line.Color != "3535FF" || len(bus.Line.Path) != 7 {
		t.Fatalf("line %+v", bus.Line)
	}
	if bus.Headsign != "Hamden Plaza" || bus.StopCount != 3 || bus.CostCents != fareCents || first.CostCents() != fareCents {
		t.Fatalf("bus %q %d %d", bus.Headsign, bus.StopCount, bus.CostCents)
	}
	if len(bus.Path) != 7 || bus.Path[0] != (geo.LatLon{Lat: 41.3, Lon: -72.93}) {
		t.Fatalf("path %v", bus.Path)
	}
	if bus.Vehicle != nil {
		t.Fatal("vehicle without realtime")
	}
	second := found[1].Legs[1]
	if !second.DepartAt.Equal(at("2026-11-17 08:30:00")) || len(second.Path) != 7 {
		t.Fatalf("second %v path %d", second.DepartAt, len(second.Path))
	}
}

func TestOptionsApplyRealtime(t *testing.T) {
	departAt := at("2026-11-17 07:55:00")
	m := loadedMode(t, departAt)
	s := m.schedule.Load()
	updates, err := parseTripUpdates(tripUpdateFeed(t, departAt), s, departAt)
	if err != nil {
		t.Fatal(err)
	}
	positions, err := parseVehiclePositions(vehicleFeed(t, departAt), s, departAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates.trips) != 1 || len(positions.byTrip) != 1 {
		t.Fatalf("updates %d positions %d", len(updates.trips), len(positions.byTrip))
	}
	m.updates.Store(updates)
	m.positions.Store(positions)
	found := options(t, m, departAt)
	bus := found[0].Legs[1]
	if !bus.DepartAt.Equal(at("2026-11-17 08:02:00")) || !bus.ArriveAt.Equal(at("2026-11-17 08:12:00")) {
		t.Fatalf("bus %v %v", bus.DepartAt, bus.ArriveAt)
	}
	if !bus.ArriveAtLate.Equal(bus.ArriveAt.Add(lateMarginRealtime)) {
		t.Fatalf("late %v", bus.ArriveAtLate)
	}
	if bus.Vehicle == nil || bus.Vehicle.Name != "1432" || bus.Vehicle.Heading == nil || *bus.Vehicle.Heading != 10 {
		t.Fatalf("vehicle %+v", bus.Vehicle)
	}
	m.now = func() time.Time { return departAt.Add(3 * time.Minute) }
	stale := options(t, m, departAt)[0].Legs[1]
	if !stale.DepartAt.Equal(at("2026-11-17 08:00:00")) || stale.Vehicle != nil {
		t.Fatalf("stale realtime used %v", stale.DepartAt)
	}
}

func TestOptionsDropCanceledAndPassedTrips(t *testing.T) {
	departAt := at("2026-11-17 07:55:00")
	m := loadedMode(t, departAt)
	s := m.schedule.Load()
	canceled := gtfs.TripDescriptor_CANCELED
	feed := &gtfs.FeedMessage{
		Header: &gtfs.FeedHeader{GtfsRealtimeVersion: proto.String("2.0"), Timestamp: proto.Uint64(uint64(departAt.Unix()))},
		Entity: []*gtfs.FeedEntity{
			{Id: proto.String("1"), TripUpdate: &gtfs.TripUpdate{Trip: &gtfs.TripDescriptor{TripId: proto.String("T1"), StartDate: proto.String("20261117"), ScheduleRelationship: &canceled}}},
			{Id: proto.String("2"), TripUpdate: &gtfs.TripUpdate{
				Trip:           &gtfs.TripDescriptor{TripId: proto.String("T2"), StartDate: proto.String("20261117")},
				StopTimeUpdate: []*gtfs.TripUpdate_StopTimeUpdate{{StopSequence: proto.Uint32(3), Arrival: &gtfs.TripUpdate_StopTimeEvent{Delay: proto.Int32(0)}}},
			}},
		},
	}
	data, err := proto.Marshal(feed)
	if err != nil {
		t.Fatal(err)
	}
	updates, err := parseTripUpdates(data, s, departAt)
	if err != nil {
		t.Fatal(err)
	}
	m.updates.Store(updates)
	if found := options(t, m, departAt); len(found) != 0 {
		t.Fatalf("options %d", len(found))
	}
}

func TestOptionsAfterMidnight(t *testing.T) {
	departAt := at("2026-11-18 00:50:00")
	found := options(t, loadedMode(t, departAt), departAt)
	if len(found) != 1 {
		t.Fatalf("options %d", len(found))
	}
	bus := found[0].Legs[1]
	if !bus.DepartAt.Equal(at("2026-11-18 01:00:00")) || bus.Line.Name != "212 Grand Ave" {
		t.Fatalf("bus %v %s", bus.DepartAt, bus.Line.Name)
	}
}

func TestOptionsRespectCalendarDates(t *testing.T) {
	departAt := at("2026-11-26 07:55:00")
	if found := options(t, loadedMode(t, departAt), departAt); len(found) != 0 {
		t.Fatalf("options on a removed date %d", len(found))
	}
	saturday := at("2026-11-21 07:55:00")
	if found := options(t, loadedMode(t, saturday), saturday); len(found) != 0 {
		t.Fatalf("options on Saturday %d", len(found))
	}
}

func TestOptionsNeedSchedule(t *testing.T) {
	m := New(streets.Estimate{})
	found, err := m.Options(context.Background(), plan.Trip{Origin: origin, Destination: destination, DepartAt: at("2026-11-17 07:55:00")})
	if err != nil || found != nil {
		t.Fatalf("%v %v", found, err)
	}
}

func TestRefreshScheduleUsesIfModifiedSince(t *testing.T) {
	archive := zipFeed(t)
	modified := "Tue, 20 Oct 2026 18:54:17 GMT"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("If-Modified-Since") == modified {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Last-Modified", modified)
		w.Write(archive)
	}))
	defer server.Close()
	m := New(streets.Estimate{})
	m.scheduleURL = server.URL
	if err := m.refreshSchedule(context.Background()); err != nil {
		t.Fatal(err)
	}
	first := m.schedule.Load()
	if first == nil || len(first.trips) != 3 || first.lastModified != modified {
		t.Fatal("schedule not loaded")
	}
	if err := m.refreshSchedule(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.schedule.Load() != first || requests != 2 {
		t.Fatal("schedule reloaded although unchanged")
	}
}

func TestHeadsignAndName(t *testing.T) {
	if got := headsign("265B - Bull Hill La", "265"); got != "Bull Hill La" {
		t.Fatal(got)
	}
	if got := headsign("Downtown", "265"); got != "Downtown" {
		t.Fatal(got)
	}
	if got := lineName(route{shortName: "201", longName: "NEW HAVEN/MADISON"}); got != "201 New Haven/Madison" {
		t.Fatal(got)
	}
}

func tripUpdateFeed(t *testing.T, now time.Time) []byte {
	t.Helper()
	departure := at("2026-11-17 08:02:00").Unix()
	arrival := at("2026-11-17 08:12:00").Unix()
	feed := &gtfs.FeedMessage{
		Header: &gtfs.FeedHeader{GtfsRealtimeVersion: proto.String("2.0"), Timestamp: proto.Uint64(uint64(now.Unix()))},
		Entity: []*gtfs.FeedEntity{
			{Id: proto.String("1"), TripUpdate: &gtfs.TripUpdate{
				Trip: &gtfs.TripDescriptor{TripId: proto.String("T1"), StartDate: proto.String("20261117"), RouteId: proto.String("16228")},
				StopTimeUpdate: []*gtfs.TripUpdate_StopTimeUpdate{
					{StopSequence: proto.Uint32(1), StopId: proto.String("A"), Departure: &gtfs.TripUpdate_StopTimeEvent{Time: proto.Int64(departure)}},
					{StopSequence: proto.Uint32(4), StopId: proto.String("D"), Arrival: &gtfs.TripUpdate_StopTimeEvent{Time: proto.Int64(arrival)}},
				},
			}},
			{Id: proto.String("2"), TripUpdate: &gtfs.TripUpdate{
				Trip:           &gtfs.TripDescriptor{TripId: proto.String("H1"), RouteId: proto.String("16462")},
				StopTimeUpdate: []*gtfs.TripUpdate_StopTimeUpdate{{StopSequence: proto.Uint32(1), Arrival: &gtfs.TripUpdate_StopTimeEvent{Delay: proto.Int32(60)}}},
			}},
		},
	}
	data, err := proto.Marshal(feed)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func vehicleFeed(t *testing.T, now time.Time) []byte {
	t.Helper()
	feed := &gtfs.FeedMessage{
		Header: &gtfs.FeedHeader{GtfsRealtimeVersion: proto.String("2.0"), Timestamp: proto.Uint64(uint64(now.Unix()))},
		Entity: []*gtfs.FeedEntity{
			{Id: proto.String("1432"), Vehicle: &gtfs.VehiclePosition{
				Trip:      &gtfs.TripDescriptor{TripId: proto.String("T1")},
				Vehicle:   &gtfs.VehicleDescriptor{Id: proto.String("2432"), Label: proto.String("1432")},
				Position:  &gtfs.Position{Latitude: proto.Float32(41.299), Longitude: proto.Float32(-72.93), Bearing: proto.Float32(10)},
				Timestamp: proto.Uint64(uint64(now.Unix())),
			}},
			{Id: proto.String("9"), Vehicle: &gtfs.VehiclePosition{
				Trip:     &gtfs.TripDescriptor{TripId: proto.String("H1")},
				Position: &gtfs.Position{Latitude: proto.Float32(41.76), Longitude: proto.Float32(-72.67)},
			}},
		},
	}
	data, err := proto.Marshal(feed)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func zipFeed(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	names, err := filepath.Glob("testdata/feed/*.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		content, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		file, err := writer.Create(filepath.Base(name))
		if err != nil {
			t.Fatal(err)
		}
		file.Write(content)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
