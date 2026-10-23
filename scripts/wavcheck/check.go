package main

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bhlabz/notted/internal/integration/audio/wavinfo"
)

const chunkToleranceSeconds = 1.0

// CheckChunks returns the problems found in paths, each as "<path>: <reason>".
// With chunkSeconds <= 0 nothing is checked beyond readability.
func CheckChunks(paths []string, chunkSeconds float64) []string {
	var problems []string
	infos := map[string]wavinfo.Info{}
	for _, p := range paths {
		info, err := wavinfo.Read(p)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", p, err))
			continue
		}
		infos[p] = info
		if chunkSeconds <= 0 {
			continue
		}
		if info.SampleRate != 16000 || info.Channels != 1 || info.BitsPerSample != 16 {
			problems = append(problems, fmt.Sprintf("%s: want 16000 Hz mono 16-bit, got %d Hz %d channels %d-bit",
				p, info.SampleRate, info.Channels, info.BitsPerSample))
		}
	}
	if chunkSeconds <= 0 {
		return problems
	}

	groups := map[string][]string{}
	for p := range infos {
		base := filepath.Base(p)
		name := strings.TrimSuffix(base, filepath.Ext(base))
		prefix := name
		if i := strings.LastIndex(name, "-"); i >= 0 {
			prefix = name[:i]
		}
		key := filepath.Dir(p) + "\x00" + prefix
		groups[key] = append(groups[key], p)
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		files := groups[k]
		sort.Strings(files)
		for i, p := range files {
			d := infos[p].DurationSeconds
			if i < len(files)-1 {
				if math.Abs(d-chunkSeconds) > chunkToleranceSeconds {
					problems = append(problems, fmt.Sprintf("%s: duration %.2fs, want %.2fs +/- %.1fs", p, d, chunkSeconds, chunkToleranceSeconds))
				}
			} else if d <= 0 || d > chunkSeconds+chunkToleranceSeconds {
				problems = append(problems, fmt.Sprintf("%s: last chunk duration %.2fs, want above 0 and at most %.2fs", p, d, chunkSeconds+chunkToleranceSeconds))
			}
		}
	}
	return problems
}
