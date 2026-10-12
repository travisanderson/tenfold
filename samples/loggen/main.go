// Command loggen writes synthetic Zscaler-style web-proxy logs for a
// fictional company, with planted attacks, benign decoys, and an answer key
// that lists the lines of each.
//
// Run it from the samples folder:
//
//	go run ./loggen --all
//	go run ./loggen --profile attack-day --seed 42
//	go run ./loggen --profile holdout --seed 20261012 --out holdout
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func main() {
	profile := flag.String("profile", "", "profile to write: clean-day, attack-day, large-100k or holdout")
	all := flag.Bool("all", false, "write every demo profile; holdout is left out")
	seed := flag.Uint64("seed", 42, "random seed; the same seed gives identical files")
	out := flag.String("out", ".", "output directory")
	date := flag.String("date", "2026-10-08", "the Eastern calendar day the log covers")
	flag.Parse()

	day, err := time.Parse("2006-01-02", *date)
	if err != nil {
		fail("--date must look like 2026-10-08")
	}

	var chosen []Profile
	for _, p := range profiles {
		if (*all && !p.Holdout) || p.Name == *profile {
			chosen = append(chosen, p)
		}
	}
	if len(chosen) == 0 {
		fail("choose --all or --profile clean-day | attack-day | large-100k | holdout")
	}

	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err.Error())
	}
	for _, p := range chosen {
		log, key := Generate(p, *seed, day)
		logPath := filepath.Join(*out, p.Name+".log")
		if err := os.WriteFile(logPath, log, 0o644); err != nil {
			fail(err.Error())
		}
		if err := os.WriteFile(filepath.Join(*out, p.Name+".answers.json"), MarshalKey(key), 0o644); err != nil {
			fail(err.Error())
		}
		fmt.Printf("%s: %d lines, %.1f MB, %d scenarios, %d malformed\n",
			logPath, key.LineCount, float64(len(log))/(1<<20), len(key.Scenarios), len(key.MalformedLines))
	}
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "loggen: "+msg)
	os.Exit(2)
}
