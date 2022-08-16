package main

import (
	"fmt"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
)

type Manifest struct {
	Name      string              `yaml:"name"`
	Modules   []Module            `yaml:"modules"`
	Arguments map[string]Argument `yaml:"arguments"`
}

type Module struct {
	Name string `yaml:"name"`
}

type Argument struct {
	From string `yaml:"from"`
}

type ConfigArgument struct {
	DependentOn []Dependent `yaml:"dependentOn"`
}

type Dependent struct {
	Module   string `yaml:"module"`
	Argument string `yaml:"argument"`
	Expr     string `yaml:"expr"`
}

func containsValue(key string, list []string) bool {
	contained := false
	for _, v := range list {
		if key == v {
			contained = true
			break
		}
	}
	return contained
}

func checkKeys(manifest Manifest, config map[string]ConfigArgument) string {
	var manifestKeys []string
	for k := range manifest.Arguments {
		manifestKeys = append(manifestKeys, k)
	}

	var configKeys []string
	for k := range config {
		configKeys = append(configKeys, k)
	}

	for _, key := range configKeys {
		if !containsValue(key, manifestKeys) {
			return fmt.Sprintf("key %s is in stork.yaml but not in manifest.yaml", key)
		}
	}

	for k, v := range config {
		for _, dep := range v.DependentOn {
			if dep.Argument != "" && dep.Module == "" && !containsValue(dep.Argument, manifestKeys) {
				return fmt.Sprintf("key %s is a dependency for %s in stork.yaml but not in manifest.yaml", dep.Argument, k)
			}
		}
	}

	return ""
}

func checkModules(manifest Manifest, config map[string]ConfigArgument) string {
	var importedModules []string
	for _, v := range manifest.Modules {
		importedModules = append(importedModules, v.Name)
	}

	for _, v := range manifest.Arguments {
		if v.From != "" {
			if !containsValue(v.From, importedModules) {
				return fmt.Sprintf("module %s is imported from but not declared in the modules field", v.From)
			}
		}
	}

	for k, v := range config {
		for _, dep := range v.DependentOn {
			if dep.Module != "" {
				if !containsValue(dep.Module, importedModules) {
					return fmt.Sprintf("%s depends on an argument fomr %s but it is not declared in the modules field", k, dep.Module)
				}
			}
		}
	}

	return ""
}

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	manifestText, err := os.ReadFile("manifest.yaml")
	if err != nil {
		// This is a repo without a manifest file so just ignore it
		log.Info().Msg("unable to read manifest.yaml file, assuming there is not one, skipping test")
		return
	}

	configText, err := os.ReadFile("stork.yaml")
	if err != nil {
		// This is a repo without a stork config so just ignore it
		log.Info().Msg("unable to read stork.yaml file, assuming there is not one")
		configText = []byte{}
	}

	var manifest Manifest

	err = yaml.Unmarshal(manifestText, &manifest)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to parse manifest.yaml file")
	}

	config := make(map[string]ConfigArgument)

	if len(configText) != 0 {
		err = yaml.Unmarshal(configText, &config)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to parse stork.yaml file")
		}
	} else {
		log.Info().Msg("stork.yaml is empty or does not exist")
	}

	log.Info().Msg("checking stork.yaml keys")
	if msg := checkKeys(manifest, config); msg != "" {
		log.Fatal().Msg(msg)
	}
	log.Info().Msg("stork.yaml keys only reference existent keys")

	log.Info().Msg("checking stork.yaml and manifest.yaml imports")
	if msg := checkModules(manifest, config); msg != "" {
		log.Fatal().Msg(msg)
	}
	log.Info().Msg("all stork.yaml and manifest.yaml imports and declared in the modules field")
	log.Info().Msg("all tests passed")
}
