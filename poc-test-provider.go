// POC: Test provider network connectivity
// This program tests if we can connect to a containerized provider via localhost
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/lib"
	"github.com/sirupsen/logrus"
)

func main() {
	// Setup logging
	logrusLog := logrus.New()
	logrusLog.SetOutput(os.Stdout)
	logrusLog.SetFormatter(&logrus.TextFormatter{})
	logrusLog.SetLevel(logrus.DebugLevel)

	logger := logr.Discard()

	fmt.Println("=== Provider Network Connectivity POC ===\n")

	// Test configuration: Connect to provider on localhost:9001
	// Assumes provider container is already running with --network=host --port=9001
	testConfig := provider.Config{
		Name:       "java",
		BinaryPath: "", // Empty = network mode
		Address:    "localhost:9001", // Network address
		InitConfig: []provider.InitConfig{
			{
				Location:     "/tmp", // Dummy location for testing
				AnalysisMode: provider.FullAnalysisMode,
			},
		},
	}

	fmt.Printf("Attempting to connect to provider at %s...\n", testConfig.Address)

	// Try to create provider client
	client, err := lib.GetProviderClient(testConfig, logger)
	if err != nil {
		log.Fatalf("❌ Failed to create provider client: %v\n", err)
	}
	fmt.Println("✅ Provider client created successfully")

	// Try to initialize provider
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Println("\nInitializing provider...")
	_, err = client.ProviderInit(ctx, nil)
	if err != nil {
		log.Fatalf("❌ Failed to initialize provider: %v\n", err)
	}
	fmt.Println("✅ Provider initialized successfully")

	// Try to get capabilities
	fmt.Println("\nTesting provider capabilities...")
	caps := client.Capabilities()
	fmt.Printf("✅ Provider has %d capabilities\n", len(caps))

	if len(caps) > 0 {
		fmt.Println("\nSample capabilities:")
		for i, cap := range caps {
			if i >= 5 {
				fmt.Printf("... and %d more\n", len(caps)-5)
				break
			}
			fmt.Printf("  - %s\n", cap.Name)
		}
	}

	// Cleanup
	fmt.Println("\nCleaning up...")
	client.Stop()
	fmt.Println("✅ Provider stopped successfully")

	fmt.Println("\n=== POC Test PASSED ===")
	fmt.Println("Network-based provider communication is working!")
}
