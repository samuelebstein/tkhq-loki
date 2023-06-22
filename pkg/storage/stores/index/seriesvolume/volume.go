package seriesvolume

import (
	"sort"
	"sync"
	"time"

	"github.com/prometheus/common/model"

	"github.com/grafana/loki/pkg/logproto"
)

const (
	MatchAny     = "{}"
	DefaultLimit = 100
)

// TODO(masslessparticle): Lock striping to reduce contention on this map
type Accumulator struct {
	lock    sync.RWMutex
	volumes map[string]uint64
	limit   int32
}

func NewAccumulator(limit int32) *Accumulator {
	return &Accumulator{
		volumes: make(map[string]uint64),
		limit:   limit,
	}
}

func (acc *Accumulator) AddVolumes(volumes map[string]uint64) {
	acc.lock.Lock()
	defer acc.lock.Unlock()

	for name, size := range volumes {
		acc.volumes[name] += size
	}
}

func (acc *Accumulator) Volumes(from, through time.Time) *logproto.VolumeResponse {
	acc.lock.RLock()
	defer acc.lock.RUnlock()

	return MapToSeriesVolumeResponse(acc.volumes, int(acc.limit), from, through)
}

func Merge(responses []*logproto.VolumeResponse, limit int32) *logproto.VolumeResponse {
	mergedVolumes := make(map[string]uint64)
	var from, through time.Time

	for _, res := range responses {
		if res == nil {
			// Some stores return nil responses
			continue
		}

		resFrom, resThrough := res.From.Time(), res.Through.Time()

		if resFrom.Before(from) || from.IsZero() {
			from = resFrom
		}

		if resThrough.After(through) || through.IsZero() {
			through = resThrough
		}

		for _, v := range res.Volumes {
			mergedVolumes[v.Name] += v.GetVolume()

		}
	}

	return MapToSeriesVolumeResponse(mergedVolumes, int(limit), from, through)
}

func MapToSeriesVolumeResponse(mergedVolumes map[string]uint64, limit int, from, through time.Time) *logproto.VolumeResponse {
	volumes := make([]logproto.Volume, 0, len(mergedVolumes))
	for name, size := range mergedVolumes {
		volumes = append(volumes, logproto.Volume{
			Name:   name,
			Volume: size,
		})
	}

	sort.Slice(volumes, func(i, j int) bool {
		if volumes[i].Volume == volumes[j].Volume {
			return volumes[i].Name < volumes[j].Name
		}

		return volumes[i].Volume > volumes[j].Volume
	})

	if limit < len(volumes) {
		volumes = volumes[:limit]
	}

	return &logproto.VolumeResponse{
		Volumes: volumes,
		Limit:   int32(limit),
		From:    model.TimeFromUnixNano(from.UnixNano()),
		Through: model.TimeFromUnixNano(through.UnixNano()),
	}
}
