package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/temporalio/s2s-proxy/config"
	"gopkg.in/yaml.v3"
)

// NewFormatConfig contains only the non-deprecated fields from S2SProxyConfig.
// This excludes fields marked as "TODO: Soon to be deprecated! Create an item in ClusterConnections instead"
type NewFormatConfig struct {
	NamespaceNameTranslation   config.NameTranslationConfig    `yaml:"namespaceNameTranslation,omitempty"`
	SearchAttributeTranslation config.SATranslationConfig      `yaml:"searchAttributeTranslation,omitempty"`
	Metrics                    *config.MetricsConfig           `yaml:"metrics,omitempty"`
	ProfilingConfig            config.ProfilingConfig          `yaml:"profiling,omitempty"`
	Logging                    config.LoggingConfig            `yaml:"logging,omitempty"`
	LogConfigs                 map[string]config.LoggingConfig `yaml:"logConfigs,omitempty"`
	ClusterConnections         []config.ClusterConnConfig      `yaml:"clusterConnections,omitempty"`
}

func main() {
	var inputPath string
	var outputPath string

	flag.StringVar(&inputPath, "input", "", "Path to input config file (YAML format)")
	flag.StringVar(&outputPath, "output", "", "Path to output config file (YAML format)")
	flag.Parse()

	if inputPath == "" {
		log.Fatal("Error: -input flag is required")
	}
	if outputPath == "" {
		log.Fatal("Error: -output flag is required")
	}

	// Load the old config
	fmt.Printf("Reading config from: %s\n", inputPath)
	oldConfig, err := config.LoadConfig[config.S2SProxyConfig](inputPath)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	// Convert to new format
	fmt.Println("Converting config to new format...")
	convertedConfig := config.ToClusterConnConfig(oldConfig)

	// Create output config with only non-deprecated fields
	outputConfig := NewFormatConfig{
		NamespaceNameTranslation:   convertedConfig.NamespaceNameTranslation,
		SearchAttributeTranslation: convertedConfig.SearchAttributeTranslation,
		Metrics:                    convertedConfig.Metrics,
		ProfilingConfig:            convertedConfig.ProfilingConfig,
		Logging:                    convertedConfig.Logging,
		LogConfigs:                 convertedConfig.LogConfigs,
		ClusterConnections:         convertedConfig.ClusterConnections,
	}

	// Write the new config
	fmt.Printf("Writing converted config to: %s\n", outputPath)
	data, err := yaml.Marshal(&outputConfig)
	if err != nil {
		log.Fatalf("Error marshaling config: %v", err)
	}

	err = os.WriteFile(outputPath, data, 0644)
	if err != nil {
		log.Fatalf("Error writing config: %v", err)
	}

	fmt.Println("Config conversion completed successfully!")
	os.Exit(0)
}
