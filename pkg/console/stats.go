/*
 * Copyright (c) 2025-2026 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package console

import (
	"expvar"
	"net/http"
	"sort"
	"sync"
	"time"

	"go.arpabet.com/consensusdb/pkg/pb"
	"go.uber.org/zap"
)

/*
Dashboard data: the regions with their at-rest (on-disk) and on-transfer
(logical) sizes, plus store-wide throughput. Throughput is exposed as cumulative
read/write counters (from badger's metrics) and on-disk sizes; the client
computes reads/writes per second from the delta between two polls, so the server
stays stateless and cheap. The region breakdown is a full scan, so it is cached
briefly to bound the cost of dashboard refreshes.
*/

// RegionStat is one (tenant, region)'s footprint.
type RegionStat struct {
	Tenant       string `json:"tenant"`
	Region       string `json:"region"`
	Keys         int64  `json:"keys"`
	RestBytes    int64  `json:"restBytes"`     // on-disk size (compressed/encrypted)
	TransferByte int64  `json:"transferBytes"` // logical value bytes (what crosses the wire)
}

const regionsCacheTTL = 10 * time.Second

// regionsCache memoizes the scan result briefly.
type regionsCache struct {
	mu       sync.Mutex
	at       time.Time
	regions  []RegionStat
	scanning bool
}

// stats returns store-wide throughput counters and on-disk size. The client turns
// the cumulative reads/writes into per-second rates.
func (t *ConsoleHandler) stats(w http.ResponseWriter) {
	out := map[string]any{
		"reads":  expvarInt("badger_get_num_user"),
		"writes": expvarInt("badger_put_num_user"),
		"unix":   time.Now().UnixMilli(),
	}
	if sizer, ok := t.Storage.(interface {
		DiskSize() (int64, int64)
	}); ok {
		lsm, vlog := sizer.DiskSize()
		out["diskLsmBytes"] = lsm
		out["diskVlogBytes"] = vlog
		out["diskBytes"] = lsm + vlog
	}
	writeJSON(w, http.StatusOK, out)
}

// regions returns the per-region footprint, served from a short-lived cache.
func (t *ConsoleHandler) regions(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]any{"regions": t.cachedRegions()})
}

func (t *ConsoleHandler) cachedRegions() []RegionStat {
	t.regionsCache.mu.Lock()
	fresh := time.Since(t.regionsCache.at) < regionsCacheTTL && t.regionsCache.regions != nil
	cached := t.regionsCache.regions
	t.regionsCache.mu.Unlock()
	if fresh {
		return cached
	}

	agg := map[string]*RegionStat{}
	sender := senderFunc(func(block *pb.Block) error {
		for _, rec := range block.Record {
			if rec == nil || rec.Key == nil {
				continue
			}
			tenant, region := string(rec.Key.MajorKey), string(rec.Key.RegionName)
			key := tenant + "\x00" + region
			s := agg[key]
			if s == nil {
				s = &RegionStat{Tenant: tenant, Region: region}
				agg[key] = s
			}
			s.Keys++
			s.TransferByte += int64(len(rec.Value))
			if rec.Head != nil {
				s.RestBytes += rec.Head.DiskSize
			}
		}
		return nil
	})
	// Full scan (values included for the logical size); bounded by the cache above.
	if err := t.Storage.Scan(&pb.ScanRequest{}, sender); err != nil {
		t.Log.Warn("RegionsScan", zap.Error(err))
	}

	out := make([]RegionStat, 0, len(agg))
	for _, s := range agg {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tenant != out[j].Tenant {
			return out[i].Tenant < out[j].Tenant
		}
		return out[i].Region < out[j].Region
	})

	t.regionsCache.mu.Lock()
	t.regionsCache.regions = out
	t.regionsCache.at = time.Now()
	t.regionsCache.mu.Unlock()
	return out
}

// expvarInt reads a cumulative badger counter (0 when absent).
func expvarInt(name string) int64 {
	if v, ok := expvar.Get(name).(*expvar.Int); ok && v != nil {
		return v.Value()
	}
	return 0
}
