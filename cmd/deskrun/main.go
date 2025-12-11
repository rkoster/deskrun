package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/rkoster/deskrun/internal/cmd"
	"k8s.io/klog/v2"
)

func init() {
	// Initialize klog flags and set the verbosity level to 0 to suppress verbose logs
	klog.InitFlags(nil)

	// Set the log level to 0 to suppress info-level throttling messages
	flag.Set("v", "0")
	flag.Set("logtostderr", "false")
	flag.Set("alsologtostderr", "false")
	flag.Set("stderrthreshold", "2") // Only log ERROR and FATAL to stderr
}

func main() {
	// Parse flags to apply the klog settings
	flag.Parse()

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
