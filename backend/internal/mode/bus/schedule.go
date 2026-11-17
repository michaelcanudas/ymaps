package bus

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"ymaps/backend/internal/geo"
)

const (
	newHavenAgency = "2"
	defaultColor   = "1A5FB4"
)

var newYork = mustLoadLocation("America/New_York")

func mustLoadLocation(name string) *time.Location {
	location, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return location
}

type stop struct {
	id   string
	name string
	at   geo.LatLon
}

type route struct {
	id        string
	shortName string
	longName  string
	color     string
}

type shape struct {
	points []geo.LatLon
	dist   []float32
}

type trip struct {
	id       string
	route    int32
	service  int32
	shape    int32
	headsign string
	first    int32
	count    int32
}

const (
	noPickup  uint8 = 1
	noDropOff uint8 = 2
)

type stopTime struct {
	stop      int32
	arrival   int32
	departure int32
	sequence  uint32
	dist      float32
	flags     uint8
}

type visit struct {
	trip     int32
	position int32
}

type service struct {
	weekdays   uint8
	start      int32
	end        int32
	exceptions map[int32]bool
}

type schedule struct {
	lastModified string
	loadedAt     time.Time
	stops        []stop
	routes       []route
	shapes       []shape
	trips        []trip
	tripIndex    map[string]int32
	times        []stopTime
	visitStart   []int32
	visits       []visit
	services     []service
}

type row struct {
	columns map[string]int
	record  []string
}

func (r row) get(name string) string {
	index, ok := r.columns[name]
	if !ok || index >= len(r.record) {
		return ""
	}
	return strings.TrimSpace(r.record[index])
}

func readTable(fsys fs.FS, name string, optional bool, each func(row) error) error {
	file, err := fsys.Open(name)
	if err != nil {
		if optional && errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	defer file.Close()
	reader := csv.NewReader(bufio.NewReaderSize(file, 1<<16))
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	reader.ReuseRecord = true
	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	columns := make(map[string]int, len(header))
	for index, column := range header {
		columns[strings.TrimSpace(strings.TrimPrefix(column, "\uFEFF"))] = index
	}
	for {
		record, err := reader.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if err := each(row{columns: columns, record: record}); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
}

func parseSchedule(fsys fs.FS) (*schedule, error) {
	s := &schedule{tripIndex: map[string]int32{}}
	routeIndex := map[string]int32{}
	serviceIndex := map[string]int32{}
	shapeIndex := map[string]int32{}
	stopIndex := map[string]int32{}

	err := readTable(fsys, "routes.txt", false, func(r row) error {
		if r.get("agency_id") != newHavenAgency {
			return nil
		}
		id := strings.Clone(r.get("route_id"))
		routeIndex[id] = int32(len(s.routes))
		s.routes = append(s.routes, route{
			id:        id,
			shortName: strings.Clone(r.get("route_short_name")),
			longName:  strings.Clone(r.get("route_long_name")),
			color:     validColor(r.get("route_color")),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	err = readTable(fsys, "trips.txt", false, func(r row) error {
		routeID, ok := routeIndex[r.get("route_id")]
		if !ok {
			return nil
		}
		serviceID := r.get("service_id")
		serviceNumber, ok := serviceIndex[serviceID]
		if !ok {
			serviceNumber = int32(len(serviceIndex))
			serviceIndex[strings.Clone(serviceID)] = serviceNumber
		}
		shapeNumber := int32(-1)
		if shapeID := r.get("shape_id"); shapeID != "" {
			number, ok := shapeIndex[shapeID]
			if !ok {
				number = int32(len(shapeIndex))
				shapeIndex[strings.Clone(shapeID)] = number
			}
			shapeNumber = number
		}
		id := strings.Clone(r.get("trip_id"))
		s.tripIndex[id] = int32(len(s.trips))
		s.trips = append(s.trips, trip{
			id:       id,
			route:    routeID,
			service:  serviceNumber,
			shape:    shapeNumber,
			headsign: strings.Clone(r.get("trip_headsign")),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(s.trips) == 0 {
		return nil, errors.New("no New Haven trips in feed")
	}

	perTrip := make([][]stopTime, len(s.trips))
	err = readTable(fsys, "stop_times.txt", false, func(r row) error {
		tripNumber, ok := s.tripIndex[r.get("trip_id")]
		if !ok {
			return nil
		}
		stopID := r.get("stop_id")
		stopNumber, ok := stopIndex[stopID]
		if !ok {
			stopNumber = int32(len(stopIndex))
			stopIndex[strings.Clone(stopID)] = stopNumber
		}
		sequence, err := strconv.ParseUint(r.get("stop_sequence"), 10, 32)
		if err != nil {
			return err
		}
		arrival := parseClock(r.get("arrival_time"))
		departure := parseClock(r.get("departure_time"))
		if arrival < 0 {
			arrival = departure
		}
		if departure < 0 {
			departure = arrival
		}
		var flags uint8
		if r.get("pickup_type") == "1" {
			flags |= noPickup
		}
		if r.get("drop_off_type") == "1" {
			flags |= noDropOff
		}
		perTrip[tripNumber] = append(perTrip[tripNumber], stopTime{
			stop:      stopNumber,
			arrival:   arrival,
			departure: departure,
			sequence:  uint32(sequence),
			dist:      parseDistance(r.get("shape_dist_traveled")),
			flags:     flags,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	total := 0
	for _, times := range perTrip {
		total += len(times)
	}
	s.times = make([]stopTime, 0, total)
	for number, times := range perTrip {
		sort.Slice(times, func(i, j int) bool { return times[i].sequence < times[j].sequence })
		interpolateTimes(times)
		if len(times) > 0 && isMissing(times[0].dist) {
			times[0].dist = 0
		}
		s.trips[number].first = int32(len(s.times))
		s.trips[number].count = int32(len(times))
		s.times = append(s.times, times...)
		perTrip[number] = nil
	}

	s.stops = make([]stop, len(stopIndex))
	for id, number := range stopIndex {
		s.stops[number].id = id
	}
	err = readTable(fsys, "stops.txt", false, func(r row) error {
		number, ok := stopIndex[r.get("stop_id")]
		if !ok {
			return nil
		}
		lat, latErr := strconv.ParseFloat(r.get("stop_lat"), 64)
		lon, lonErr := strconv.ParseFloat(r.get("stop_lon"), 64)
		if latErr != nil || lonErr != nil {
			return nil
		}
		s.stops[number].name = strings.Clone(r.get("stop_name"))
		s.stops[number].at = geo.LatLon{Lat: lat, Lon: lon}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := s.readShapes(fsys, shapeIndex); err != nil {
		return nil, err
	}
	if err := s.readServices(fsys, serviceIndex); err != nil {
		return nil, err
	}
	s.indexVisits()
	return s, nil
}

func (s *schedule) readShapes(fsys fs.FS, shapeIndex map[string]int32) error {
	type shapePoint struct {
		sequence int
		at       geo.LatLon
		dist     float32
	}
	points := make([][]shapePoint, len(shapeIndex))
	err := readTable(fsys, "shapes.txt", true, func(r row) error {
		number, ok := shapeIndex[r.get("shape_id")]
		if !ok {
			return nil
		}
		sequence, err := strconv.Atoi(r.get("shape_pt_sequence"))
		if err != nil {
			return err
		}
		lat, latErr := strconv.ParseFloat(r.get("shape_pt_lat"), 64)
		lon, lonErr := strconv.ParseFloat(r.get("shape_pt_lon"), 64)
		if latErr != nil || lonErr != nil {
			return nil
		}
		points[number] = append(points[number], shapePoint{sequence: sequence, at: geo.LatLon{Lat: lat, Lon: lon}, dist: parseDistance(r.get("shape_dist_traveled"))})
		return nil
	})
	if err != nil {
		return err
	}
	s.shapes = make([]shape, len(shapeIndex))
	for number, list := range points {
		sort.Slice(list, func(i, j int) bool { return list[i].sequence < list[j].sequence })
		path := make([]geo.LatLon, len(list))
		dist := make([]float32, len(list))
		complete := len(list) > 0
		for i, point := range list {
			path[i] = point.at
			dist[i] = point.dist
			if isMissing(point.dist) {
				complete = false
			}
		}
		if !complete {
			dist = nil
		}
		s.shapes[number] = shape{points: path, dist: dist}
	}
	return nil
}

func (s *schedule) readServices(fsys fs.FS, serviceIndex map[string]int32) error {
	s.services = make([]service, len(serviceIndex))
	days := []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}
	err := readTable(fsys, "calendar.txt", true, func(r row) error {
		number, ok := serviceIndex[r.get("service_id")]
		if !ok {
			return nil
		}
		var weekdays uint8
		for index, day := range days {
			if r.get(day) == "1" {
				weekdays |= 1 << index
			}
		}
		start, _ := strconv.Atoi(r.get("start_date"))
		end, _ := strconv.Atoi(r.get("end_date"))
		s.services[number].weekdays = weekdays
		s.services[number].start = int32(start)
		s.services[number].end = int32(end)
		return nil
	})
	if err != nil {
		return err
	}
	return readTable(fsys, "calendar_dates.txt", true, func(r row) error {
		number, ok := serviceIndex[r.get("service_id")]
		if !ok {
			return nil
		}
		date, err := strconv.Atoi(r.get("date"))
		if err != nil {
			return nil
		}
		if s.services[number].exceptions == nil {
			s.services[number].exceptions = map[int32]bool{}
		}
		switch r.get("exception_type") {
		case "1":
			s.services[number].exceptions[int32(date)] = true
		case "2":
			s.services[number].exceptions[int32(date)] = false
		}
		return nil
	})
}

func (s *schedule) indexVisits() {
	counts := make([]int32, len(s.stops)+1)
	for _, time := range s.times {
		counts[time.stop+1]++
	}
	for i := 1; i < len(counts); i++ {
		counts[i] += counts[i-1]
	}
	s.visitStart = counts
	s.visits = make([]visit, len(s.times))
	fill := make([]int32, len(s.stops))
	copy(fill, counts[:len(s.stops)])
	for tripNumber, trip := range s.trips {
		for position := trip.first; position < trip.first+trip.count; position++ {
			stopNumber := s.times[position].stop
			s.visits[fill[stopNumber]] = visit{trip: int32(tripNumber), position: position}
			fill[stopNumber]++
		}
	}
	for stopNumber := range s.stops {
		segment := s.visits[s.visitStart[stopNumber]:s.visitStart[stopNumber+1]]
		sort.Slice(segment, func(i, j int) bool {
			return s.times[segment[i].position].departure < s.times[segment[j].position].departure
		})
	}
}

func (s *schedule) stopVisits(stopNumber int32) []visit {
	return s.visits[s.visitStart[stopNumber]:s.visitStart[stopNumber+1]]
}

func (s *schedule) tripTimes(t trip) []stopTime {
	return s.times[t.first : t.first+t.count]
}

func (s *schedule) positionOfSequence(t trip, sequence uint32) (int32, bool) {
	times := s.tripTimes(t)
	index := sort.Search(len(times), func(i int) bool { return times[i].sequence >= sequence })
	if index < len(times) && times[index].sequence == sequence {
		return t.first + int32(index), true
	}
	return 0, false
}

func (s *schedule) active(serviceNumber int32, day serviceDay) bool {
	service := s.services[serviceNumber]
	if added, ok := service.exceptions[day.date]; ok {
		return added
	}
	if day.date < service.start || day.date > service.end {
		return false
	}
	return service.weekdays&(1<<day.weekday) != 0
}

type serviceDay struct {
	date    int32
	weekday time.Weekday
	base    time.Time
}

func serviceDayOf(at time.Time, offsetDays int) serviceDay {
	local := at.In(newYork)
	year, month, day := local.Date()
	noon := time.Date(year, month, day+offsetDays, 12, 0, 0, 0, newYork)
	return serviceDay{
		date:    int32(noon.Year()*10000 + int(noon.Month())*100 + noon.Day()),
		weekday: noon.Weekday(),
		base:    noon.Add(-12 * time.Hour),
	}
}

func serviceDayOfDate(date string) (serviceDay, bool) {
	parsed, err := time.ParseInLocation("20060102", date, newYork)
	if err != nil {
		return serviceDay{}, false
	}
	return serviceDayOf(parsed.Add(12*time.Hour), 0), true
}

func (d serviceDay) at(seconds int32) time.Time {
	return d.base.Add(time.Duration(seconds) * time.Second)
}

func parseClock(value string) int32 {
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return -1
	}
	hours, errHours := strconv.Atoi(parts[0])
	minutes, errMinutes := strconv.Atoi(parts[1])
	seconds, errSeconds := strconv.Atoi(parts[2])
	if errHours != nil || errMinutes != nil || errSeconds != nil {
		return -1
	}
	return int32(hours*3600 + minutes*60 + seconds)
}

func parseDistance(value string) float32 {
	if value == "" {
		return float32(math.NaN())
	}
	parsed, err := strconv.ParseFloat(value, 32)
	if err != nil {
		return float32(math.NaN())
	}
	return float32(parsed)
}

func isMissing(value float32) bool {
	return value != value
}

func interpolateTimes(times []stopTime) {
	previous := -1
	for index := range times {
		if times[index].departure < 0 {
			continue
		}
		if previous >= 0 && index-previous > 1 {
			start := times[previous].departure
			span := times[index].arrival - start
			gap := int32(index - previous)
			for between := previous + 1; between < index; between++ {
				value := start + span*int32(between-previous)/gap
				times[between].arrival = value
				times[between].departure = value
			}
		}
		previous = index
	}
}

func validColor(value string) string {
	value = strings.TrimPrefix(value, "#")
	if len(value) != 6 {
		return defaultColor
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdefABCDEF", character) {
			return defaultColor
		}
	}
	return strings.ToUpper(value)
}
