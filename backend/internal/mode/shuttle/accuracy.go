package shuttle

import (
	"sync"
	"time"
)

type horizonBucket int

var horizonEdges = []time.Duration{3 * time.Minute, 6 * time.Minute, 11 * time.Minute, 21 * time.Minute}

func bucketOf(ahead time.Duration) horizonBucket {
	for i, edge := range horizonEdges {
		if ahead < edge {
			return horizonBucket(i)
		}
	}
	return horizonBucket(len(horizonEdges))
}

type prediction struct {
	madeAt time.Time
	at     time.Time
}

type arrivalKey struct {
	bus  int
	stop int
}

type errorStats struct {
	mean  time.Duration
	count int
}

const (
	predictionsKept      = 12
	predictionMaxAge     = 45 * time.Minute
	learningWeight       = 0.05
	trustedObservations  = 20
	arrivalSearchSeconds = 90
)

type Accuracy struct {
	mutex       sync.Mutex
	prior       []time.Duration
	predictions map[arrivalKey][]prediction
	stats       []errorStats
	lastStop    map[int]int
}

func NewAccuracy(prior []time.Duration) *Accuracy {
	return &Accuracy{
		prior:       prior,
		predictions: map[arrivalKey][]prediction{},
		stats:       make([]errorStats, len(horizonEdges)+1),
		lastStop:    map[int]int{},
	}
}

func (a *Accuracy) Predicted(bus, stop int, madeAt, at time.Time) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	key := arrivalKey{bus: bus, stop: stop}
	kept := a.predictions[key]
	if n := len(kept); n > 0 && kept[n-1].at.Equal(at) {
		return
	}
	kept = append(kept, prediction{madeAt: madeAt, at: at})
	if len(kept) > predictionsKept {
		kept = kept[len(kept)-predictionsKept:]
	}
	a.predictions[key] = kept
}

func (a *Accuracy) Passed(bus, stop int, at time.Time) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if a.lastStop[bus] == stop {
		return
	}
	a.lastStop[bus] = stop
	key := arrivalKey{bus: bus, stop: stop}
	for _, p := range a.predictions[key] {
		if at.Sub(p.madeAt) > predictionMaxAge || p.madeAt.After(at) {
			continue
		}
		bucket := bucketOf(p.at.Sub(p.madeAt))
		miss := at.Sub(p.at)
		stats := &a.stats[bucket]
		if stats.count == 0 {
			stats.mean = miss
		} else {
			stats.mean += time.Duration(learningWeight * float64(miss-stats.mean))
		}
		stats.count++
	}
	delete(a.predictions, key)
}

func (a *Accuracy) Correction(ahead time.Duration) time.Duration {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	return a.correction(bucketOf(ahead))
}

func (a *Accuracy) correction(bucket horizonBucket) time.Duration {
	prior := time.Duration(0)
	if int(bucket) < len(a.prior) {
		prior = a.prior[bucket]
	}
	stats := a.stats[bucket]
	if stats.count == 0 {
		return prior
	}
	trust := float64(stats.count) / trustedObservations
	if trust > 1 {
		trust = 1
	}
	return prior + time.Duration(trust*float64(stats.mean-prior))
}

type BucketReport struct {
	Below        time.Duration
	Correction   time.Duration
	Observations int
}

func (a *Accuracy) Report() []BucketReport {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	reports := make([]BucketReport, len(a.stats))
	for i := range a.stats {
		below := time.Duration(0)
		if i < len(horizonEdges) {
			below = horizonEdges[i]
		}
		reports[i] = BucketReport{Below: below, Correction: a.correction(horizonBucket(i)), Observations: a.stats[i].count}
	}
	return reports
}
