package main

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

const version = "1.0.0"

func printVersion() {
	fmt.Printf("OpenCall %s\n", version)
	fmt.Printf("  go       : %s\n", runtime.Version())
	fmt.Printf("  os/arch  : %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("  compiler : %s\n", runtime.Compiler)
	if bi, ok := debug.ReadBuildInfo(); ok {
		v := bi.Main.Version
		if v == "" || v == "(devel)" {
			v = "devel"
		}
		fmt.Printf("  module   : %s (%s)\n", bi.Main.Path, v)
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				fmt.Printf("  vcs      : %s\n", s.Value)
			case "vcs.time":
				fmt.Printf("  vcs.time : %s\n", s.Value)
			case "vcs.modified":
				if s.Value == "true" {
					fmt.Printf("  vcs      : dirty\n")
				}
			}
		}
	}
}
