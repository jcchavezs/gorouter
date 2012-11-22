package router

import (
	"encoding/json"
	metrics "github.com/rcrowley/go-metrics"
	"net/http"
	"router/stats"
	"sync"
	"time"
)

type topAppsEntry struct {
	ApplicationId     string `json:"application_id"`
	RequestsPerSecond int64  `json:"rps"`
	RequestsPerMinute int64  `json:"rpm"`
}

type varz struct {
	All  *HttpMetric `json:"all"`
	Tags struct {
		Component TaggedHttpMetric `json:"component"`
		Framework TaggedHttpMetric `json:"framework"`
		Runtime   TaggedHttpMetric `json:"runtime"`
	} `json:"tags"`

	Urls     int `json:"urls"`
	Droplets int `json:"droplets"`

	BadRequests    int     `json:"bad_requests"`
	RequestsPerSec float64 `json:"requests_per_sec"`

	TopApps []topAppsEntry `json:"top10_app_requests"`
}

type Varz struct {
	sync.Mutex

	*Registry

	varz
}

type httpMetric struct {
	Requests int64      `json:"requests"`
	Rate     [3]float64 `json:"rate"`

	Responses2xx int64              `json:"responses_2xx"`
	Responses3xx int64              `json:"responses_3xx"`
	Responses4xx int64              `json:"responses_4xx"`
	Responses5xx int64              `json:"responses_5xx"`
	ResponsesXxx int64              `json:"responses_xxx"`
	Latency      map[string]float64 `json:"latency"`
}

type HttpMetric struct {
	Requests metrics.Counter
	Rate     metrics.Meter

	Responses2xx metrics.Counter
	Responses3xx metrics.Counter
	Responses4xx metrics.Counter
	Responses5xx metrics.Counter
	ResponsesXxx metrics.Counter
	Latency      metrics.Histogram
}

func NewHttpMetric() *HttpMetric {
	x := &HttpMetric{
		Requests: metrics.NewCounter(),
		Rate:     metrics.NewMeter(),

		Responses2xx: metrics.NewCounter(),
		Responses3xx: metrics.NewCounter(),
		Responses4xx: metrics.NewCounter(),
		Responses5xx: metrics.NewCounter(),
		ResponsesXxx: metrics.NewCounter(),
		Latency:      metrics.NewHistogram(metrics.NewExpDecaySample(1028, 0.015)),
	}
	return x
}

func (x *HttpMetric) MarshalJSON() ([]byte, error) {
	y := httpMetric{}

	y.Requests = x.Requests.Count()
	y.Rate[0] = x.Rate.Rate1()
	y.Rate[1] = x.Rate.Rate5()
	y.Rate[2] = x.Rate.Rate15()

	y.Responses2xx = x.Responses2xx.Count()
	y.Responses3xx = x.Responses3xx.Count()
	y.Responses4xx = x.Responses4xx.Count()
	y.Responses5xx = x.Responses5xx.Count()
	y.ResponsesXxx = x.ResponsesXxx.Count()

	z := x.Latency.Percentiles([]float64{0.5, 0.75, 0.99})
	y.Latency = make(map[string]float64)
	y.Latency["50"] = z[0]
	y.Latency["75"] = z[1]
	y.Latency["99"] = z[2]

	return json.Marshal(y)
}

func (x *HttpMetric) CaptureRequest() {
	x.Requests.Inc(1)
	x.Rate.Mark(1)
}

func (x *HttpMetric) CaptureResponse(y *http.Response, z time.Duration) {
	var s int
	if y != nil {
		s = y.StatusCode / 100
	}

	switch s {
	case 2:
		x.Responses2xx.Inc(1)
	case 3:
		x.Responses3xx.Inc(1)
	case 4:
		x.Responses4xx.Inc(1)
	case 5:
		x.Responses5xx.Inc(1)
	default:
		x.ResponsesXxx.Inc(1)
	}

	x.Latency.Update(z.Nanoseconds())
}

type TaggedHttpMetric map[string]*HttpMetric

func NewTaggedHttpMetric() TaggedHttpMetric {
	x := make(TaggedHttpMetric)
	return x
}

func (x TaggedHttpMetric) httpMetric(t string) *HttpMetric {
	y := x[t]
	if y == nil {
		y = NewHttpMetric()
		x[t] = y
	}

	return y
}

func (x TaggedHttpMetric) CaptureRequest(t string) {
	x.httpMetric(t).CaptureRequest()
}

func (x TaggedHttpMetric) CaptureResponse(t string, y *http.Response, z time.Duration) {
	x.httpMetric(t).CaptureResponse(y, z)
}

func NewVarz() *Varz {
	x := &Varz{}

	x.All = NewHttpMetric()
	x.Tags.Component = make(map[string]*HttpMetric)
	x.Tags.Framework = make(map[string]*HttpMetric)
	x.Tags.Runtime = make(map[string]*HttpMetric)

	return x
}

func (x *Varz) MarshalJSON() ([]byte, error) {
	x.Lock()
	defer x.Unlock()

	x.varz.Urls = x.Registry.NumUris()
	x.varz.Droplets = x.Registry.NumBackends()

	x.varz.RequestsPerSec = x.varz.All.Rate.Rate1()

	x.updateTop()

	b, err := json.Marshal(x.varz)

	return b, err
}

func (x *Varz) updateTop() {
	t := time.Now().Add(-1 * time.Minute)
	y := x.Registry.TopApps.TopSince(t, 10)

	x.varz.TopApps = make([]topAppsEntry, 0)
	for _, z := range y {
		x.varz.TopApps = append(x.varz.TopApps, topAppsEntry{
			ApplicationId:     z.ApplicationId,
			RequestsPerSecond: z.Requests / int64(stats.TopAppsEntryLifetime.Seconds()),
			RequestsPerMinute: z.Requests,
		})
	}
}

func (x *Varz) CaptureBadRequest(req *http.Request) {
	x.Lock()
	defer x.Unlock()

	x.BadRequests++
}

func (x *Varz) CaptureBackendRequest(b Backend, req *http.Request) {
	x.Lock()
	defer x.Unlock()

	var t string
	var ok bool

	t, ok = b.Tags["component"]
	if ok {
		x.varz.Tags.Component.CaptureRequest(t)
	}

	t, ok = b.Tags["framework"]
	if ok {
		x.varz.Tags.Framework.CaptureRequest(t)
	}

	t, ok = b.Tags["runtime"]
	if ok {
		x.varz.Tags.Runtime.CaptureRequest(t)
	}

	x.varz.All.CaptureRequest()
}

func (x *Varz) CaptureBackendResponse(b Backend, res *http.Response, d time.Duration) {
	x.Lock()
	defer x.Unlock()

	var t string
	var ok bool

	t, ok = b.Tags["component"]
	if ok {
		x.varz.Tags.Component.CaptureResponse(t, res, d)
	}

	t, ok = b.Tags["framework"]
	if ok {
		x.varz.Tags.Framework.CaptureResponse(t, res, d)
	}

	t, ok = b.Tags["runtime"]
	if ok {
		x.varz.Tags.Runtime.CaptureResponse(t, res, d)
	}

	x.varz.All.CaptureResponse(res, d)
}
