package main

import (
	"context"
	"log"
	"os"
	"time"

	"ymaps/backend/internal/env"
	"ymaps/backend/internal/mode/bus"
	"ymaps/backend/internal/mode/gbfs"
	"ymaps/backend/internal/mode/shuttle"
	"ymaps/backend/internal/place"
	"ymaps/backend/internal/place/alias"
	"ymaps/backend/internal/place/google"
	"ymaps/backend/internal/place/yalemap"
	"ymaps/backend/internal/plan"
	"ymaps/backend/internal/server"
	"ymaps/backend/internal/streets"
	"ymaps/backend/internal/streets/valhalla"
)

func main() {
	env.Load(".env", "../.env")
	ctx := context.Background()

	var roads streets.Streets = streets.Estimate{}
	if url := os.Getenv("VALHALLA_URL"); url != "" {
		roads = streets.Fallback{Primary: valhalla.New(url), Backup: streets.Estimate{}}
	}

	yale := yalemap.New(os.Getenv("YALE_MAP_KEY"))
	go func() {
		if err := yale.Warm(ctx); err != nil {
			log.Printf("yale map: %v", err)
		}
	}()

	places := place.Chain{
		alias.New(),
		yale,
		google.New(os.Getenv("GOOGLE_MAPS_KEY")),
	}

	planner := plan.Planner{
		Streets:        roads,
		CentsPerMinute: 50,
		ModeTimeout:    3 * time.Second,
		Modes: []plan.Mode{
			shuttle.New(roads),
			bus.New(roads),
			gbfs.NewBike(roads, os.Getenv("RIDE_NEW_HAVEN_GBFS_KEY")),
			gbfs.NewVeo(roads),
		},
	}
	planner.Run(ctx)

	routes := &server.Routes{Places: places, Planner: planner}
	log.Fatal(server.Listen(listenAddress(), routes))
}

func listenAddress() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return ":" + port
}
