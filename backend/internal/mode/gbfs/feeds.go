package gbfs

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"ymaps/backend/internal/geo"
)

type flexBool bool

func (b *flexBool) UnmarshalJSON(raw []byte) error {
	switch strings.ToLower(strings.Trim(string(raw), `" `)) {
	case "true", "1":
		*b = true
	case "false", "0", "null", "":
		*b = false
	default:
		return fmt.Errorf("gbfs: not a boolean: %s", raw)
	}
	return nil
}

type flexFloat float64

func (f *flexFloat) UnmarshalJSON(raw []byte) error {
	text := strings.Trim(string(raw), `" `)
	if text == "" || text == "null" {
		*f = 0
		return nil
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return fmt.Errorf("gbfs: not a number: %s", raw)
	}
	*f = flexFloat(value)
	return nil
}

type envelope struct {
	LastUpdated int64           `json:"last_updated"`
	Data        json.RawMessage `json:"data"`
}

type discoveryFeed struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type discoveryLanguage struct {
	Feeds []discoveryFeed `json:"feeds"`
}

type stationInformationData struct {
	Stations []struct {
		StationID string    `json:"station_id"`
		Name      string    `json:"name"`
		Lat       flexFloat `json:"lat"`
		Lon       flexFloat `json:"lon"`
	} `json:"stations"`
}

type stationStatusData struct {
	Stations []struct {
		StationID         string    `json:"station_id"`
		NumBikesAvailable int       `json:"num_bikes_available"`
		IsInstalled       *flexBool `json:"is_installed"`
		IsRenting         flexBool  `json:"is_renting"`
		IsReturning       flexBool  `json:"is_returning"`
		Vehicles          []struct {
			BikeID        string   `json:"bike_id"`
			IsReserved    flexBool `json:"is_reserved"`
			IsDisabled    flexBool `json:"is_disabled"`
			VehicleTypeID string   `json:"vehicle_type_id"`
		} `json:"vehicles"`
	} `json:"stations"`
}

type freeBikeStatusData struct {
	Bikes []struct {
		BikeID             string     `json:"bike_id"`
		Lat                flexFloat  `json:"lat"`
		Lon                flexFloat  `json:"lon"`
		IsReserved         flexBool   `json:"is_reserved"`
		IsDisabled         flexBool   `json:"is_disabled"`
		VehicleTypeID      string     `json:"vehicle_type_id"`
		PricingPlanID      string     `json:"pricing_plan_id"`
		CurrentRangeMeters *flexFloat `json:"current_range_meters"`
		RentalURIs         struct {
			IOS string `json:"ios"`
		} `json:"rental_uris"`
	} `json:"bikes"`
}

type vehicleTypesData struct {
	VehicleTypes []struct {
		VehicleTypeID  string    `json:"vehicle_type_id"`
		FormFactor     string    `json:"form_factor"`
		PropulsionType string    `json:"propulsion_type"`
		Name           string    `json:"name"`
		MaxRangeMeters flexFloat `json:"max_range_meters"`
	} `json:"vehicle_types"`
}

type pricingPlansData struct {
	Plans []struct {
		PlanID        string    `json:"plan_id"`
		Price         flexFloat `json:"price"`
		IsTaxable     flexBool  `json:"is_taxable"`
		PerMinPricing []struct {
			Start    flexFloat  `json:"start"`
			Rate     flexFloat  `json:"rate"`
			Interval flexFloat  `json:"interval"`
			End      *flexFloat `json:"end"`
		} `json:"per_min_pricing"`
	} `json:"plans"`
}

type Station struct {
	ID             string
	Name           string
	At             geo.LatLon
	BikesAvailable int
	Renting        bool
	Returning      bool
}

type FreeVehicle struct {
	ID          string
	At          geo.LatLon
	TypeID      string
	PlanID      string
	RangeMeters *float64
	RentalURI   string
}

type VehicleType struct {
	ID         string
	Form       string
	Propulsion string
	Name       string
}

type PriceSegment struct {
	Start    float64
	Rate     float64
	Interval float64
	End      *float64
}

type PricingPlan struct {
	ID        string
	Price     float64
	Taxable   bool
	PerMinute []PriceSegment
}

const connecticutSalesTax = 0.0635

func (p PricingPlan) Cents(ride float64) int {
	minutes := math.Ceil(ride)
	total := p.Price
	for _, segment := range p.PerMinute {
		end := minutes
		if segment.End != nil && *segment.End < end {
			end = *segment.End
		}
		if end <= segment.Start {
			continue
		}
		interval := segment.Interval
		if interval <= 0 {
			interval = 1
		}
		total += segment.Rate * math.Ceil((end-segment.Start)/interval)
	}
	if p.Taxable {
		total *= 1 + connecticutSalesTax
	}
	return int(math.Round(total * 100))
}

func roundCoordinate(value flexFloat) float64 {
	return math.Round(float64(value)*1e6) / 1e6
}

func decodeStationInformation(raw json.RawMessage) (map[string]Station, error) {
	var data stationInformationData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	stations := make(map[string]Station, len(data.Stations))
	for _, entry := range data.Stations {
		id := strings.TrimSpace(entry.StationID)
		if id == "" {
			continue
		}
		stations[id] = Station{
			ID:   id,
			Name: strings.TrimSpace(entry.Name),
			At:   geo.LatLon{Lat: roundCoordinate(entry.Lat), Lon: roundCoordinate(entry.Lon)},
		}
	}
	return stations, nil
}

func decodeStationStatus(raw json.RawMessage, information map[string]Station) ([]Station, error) {
	var data stationStatusData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	stations := make([]Station, 0, len(data.Stations))
	for _, entry := range data.Stations {
		station, known := information[strings.TrimSpace(entry.StationID)]
		if !known {
			continue
		}
		installed := entry.IsInstalled == nil || bool(*entry.IsInstalled)
		station.Renting = installed && bool(entry.IsRenting)
		station.Returning = installed && bool(entry.IsReturning)
		station.BikesAvailable = entry.NumBikesAvailable
		if entry.Vehicles != nil {
			usable := 0
			for _, vehicle := range entry.Vehicles {
				if !vehicle.IsReserved && !vehicle.IsDisabled {
					usable++
				}
			}
			station.BikesAvailable = min(station.BikesAvailable, usable)
		}
		stations = append(stations, station)
	}
	return stations, nil
}

func decodeFreeBikeStatus(raw json.RawMessage) ([]FreeVehicle, error) {
	var data freeBikeStatusData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	vehicles := make([]FreeVehicle, 0, len(data.Bikes))
	for _, entry := range data.Bikes {
		if entry.IsDisabled || entry.IsReserved {
			continue
		}
		vehicle := FreeVehicle{
			ID:        strings.TrimSpace(entry.BikeID),
			At:        geo.LatLon{Lat: roundCoordinate(entry.Lat), Lon: roundCoordinate(entry.Lon)},
			TypeID:    strings.TrimSpace(entry.VehicleTypeID),
			PlanID:    strings.TrimSpace(entry.PricingPlanID),
			RentalURI: strings.TrimSpace(entry.RentalURIs.IOS),
		}
		if entry.CurrentRangeMeters != nil {
			meters := float64(*entry.CurrentRangeMeters)
			vehicle.RangeMeters = &meters
		}
		vehicles = append(vehicles, vehicle)
	}
	return vehicles, nil
}

func decodeVehicleTypes(raw json.RawMessage) (map[string]VehicleType, error) {
	var data vehicleTypesData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	types := make(map[string]VehicleType, len(data.VehicleTypes))
	for _, entry := range data.VehicleTypes {
		id := strings.TrimSpace(entry.VehicleTypeID)
		types[id] = VehicleType{
			ID:         id,
			Form:       strings.TrimSpace(entry.FormFactor),
			Propulsion: strings.TrimSpace(entry.PropulsionType),
			Name:       strings.TrimSpace(entry.Name),
		}
	}
	return types, nil
}

func decodePricingPlans(raw json.RawMessage) (map[string]PricingPlan, error) {
	var data pricingPlansData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	plans := make(map[string]PricingPlan, len(data.Plans))
	for _, entry := range data.Plans {
		id := strings.TrimSpace(entry.PlanID)
		plan := PricingPlan{ID: id, Price: float64(entry.Price), Taxable: bool(entry.IsTaxable)}
		for _, segment := range entry.PerMinPricing {
			converted := PriceSegment{Start: float64(segment.Start), Rate: float64(segment.Rate), Interval: float64(segment.Interval)}
			if segment.End != nil {
				end := float64(*segment.End)
				converted.End = &end
			}
			plan.PerMinute = append(plan.PerMinute, converted)
		}
		plans[id] = plan
	}
	return plans, nil
}
