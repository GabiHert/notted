// Command wavcheck prints the format, length and level of WAV files and checks
// recorded chunks against the helper's contract.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/GabiHert/notted/internal/integration/audio/wavinfo"
)

func main() {
	expect := flag.Float64("expect-chunk", 0, "expected chunk length in seconds (0 reports only)")
	flag.Parse()
	paths := flag.Args()
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "usage: wavcheck [-expect-chunk N] <files>")
		os.Exit(1)
	}

	failed := false
	for _, p := range paths {
		info, err := wavinfo.Read(p)
		if err != nil {
			fmt.Printf("%s  unreadable: %v\n", p, err)
			failed = true
			continue
		}
		fmt.Printf("%s  rate=%d channels=%d bits=%d duration=%.2fs rms_dbfs=%.1f silent=%t\n",
			p, info.SampleRate, info.Channels, info.BitsPerSample, info.DurationSeconds, info.RMSdBFS, info.Silent)
	}
	if *expect > 0 {
		problems := CheckChunks(paths, *expect)
		for _, pr := range problems {
			fmt.Printf("FAIL %s\n", pr)
		}
		if len(problems) > 0 {
			fmt.Printf("FAIL %d problems\n", len(problems))
			os.Exit(1)
		}
		if !failed {
			fmt.Printf("OK %d files\n", len(paths))
		}
	}
	if failed {
		os.Exit(1)
	}
}
