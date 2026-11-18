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
	"ymaps/backend/internal/place/course"
	"ymaps/backend/internal/place/google"
	"ymaps/backend/internal/place/yalemap"
	"ymaps/backend/internal/plan"
	"ymaps/backend/internal/server"
	"ymaps/backend/internal/streets"
	"ymaps/backend/internal/streets/valhalla"
	"ymaps/backend/internal/veospots"
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
		course.New(yale),
		yale,
		google.New(os.Getenv("GOOGLE_MAPS_KEY")),
	}

	veoSpots := veospots.New(os.Getenv("VEO_SPOTS_PATH"))
	veo := gbfs.NewVeo(veoSpots)

	planner := plan.Planner{
		Streets:     roads,
		ModeTimeout: 3 * time.Second,
		Modes: []plan.Mode{
			shuttle.New(),
			bus.New(),
			gbfs.NewBike(os.Getenv("RIDE_NEW_HAVEN_GBFS_KEY")),
			veo,
		},
	}
	planner.Run(ctx)
	go veoSpots.Run(ctx, veo.Reader())

	routes := &server.Routes{Places: places, Planner: planner, Debug: veoSpots.DebugHandler()}
	log.Fatal(server.Listen(listenAddress(), routes))
}

func listenAddress() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return ":" + port
}
