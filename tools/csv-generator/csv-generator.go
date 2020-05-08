package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/blang/semver"

	"github.com/openshift-kni/performance-addon-operators/pkg/utils/csvtools"

	csvv1 "github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/operators/v1alpha1"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/lib/version"
)

var (
	csvVersion          = flag.String("csv-version", "", "the unified CSV version")
	replacesCsvVersion  = flag.String("replaces-csv-version", "", "the unified CSV version this new CSV will replace")
	skipRange           = flag.String("skip-range", "", "the CSV version skip range")
	operatorCSVTemplate = flag.String("operator-csv-template-file", "", "path to csv template example")

	operatorImage = flag.String("operator-image", "", "operator container image")

	inputManifestsDir = flag.String("manifests-directory", "", "The directory containing the extra manifests to be included in the registry bundle")

	outputDir = flag.String("olm-bundle-directory", "", "The directory to output the unified CSV and CRDs to")

	annotationsFile = flag.String("inject-annotations-from", "", "inject metadata annotations from given file")
	maintainersFile = flag.String("maintainers-from", "", "add maintainers list from given file")

	semverVersion *semver.Version
)

func finalizedCsvFilename() string {
	return "performance-addon-operator.v" + *csvVersion + ".clusterserviceversion.yaml"
}

func copyFile(src string, dst string) {
	srcFile, err := os.Open(src)
	if err != nil {
		panic(err)
	}
	defer srcFile.Close()

	outFile, err := os.Create(dst)
	if err != nil {
		panic(err)
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, srcFile)
	if err != nil {
		panic(err)
	}
}

func generateUnifiedCSV(extraAnnotations, maintainers map[string]string) {

	operatorCSV := csvtools.UnmarshalCSV(*operatorCSVTemplate)

	strategySpec := csvtools.UnmarshalStrategySpec(operatorCSV)

	// this forces us to update this logic if another deployment is introduced.
	if len(strategySpec.Deployments) != 1 {
		panic(fmt.Errorf("expected 1 deployment, found %d", len(strategySpec.Deployments)))
	}

	strategySpec.Deployments[0].Spec.Template.Spec.Containers[0].Image = *operatorImage

	// Inject deplay names and descriptions for our crds
	for i, definition := range operatorCSV.Spec.CustomResourceDefinitions.Owned {
		switch definition.Name {
		case "performanceprofiles.performance.openshift.io":
			operatorCSV.Spec.CustomResourceDefinitions.Owned[i].DisplayName = "Performance Profile"
		}
	}

	// Re-serialize deployments and permissions into csv strategy.
	updatedStrat, err := json.Marshal(strategySpec)
	if err != nil {
		panic(err)
	}
	operatorCSV.Spec.InstallStrategy.StrategySpecRaw = updatedStrat

	operatorCSV.Annotations["containerImage"] = *operatorImage
	for key, value := range extraAnnotations {
		operatorCSV.Annotations[key] = value
	}

	// Set correct csv versions and name
	v := version.OperatorVersion{Version: *semverVersion}
	operatorCSV.Spec.Version = v
	operatorCSV.Name = "performance-addon-operator.v" + *csvVersion
	if *replacesCsvVersion != "" {
		operatorCSV.Spec.Replaces = "performance-addon-operator.v" + *replacesCsvVersion
	} else {
		operatorCSV.Spec.Replaces = ""
	}

	// Set api maturity
	operatorCSV.Spec.Maturity = "alpha"

	// Set links
	operatorCSV.Spec.Links = []csvv1.AppLink{
		{
			Name: "Source Code",
			URL:  "https://github.com/openshift-kni/performance-addon-operators",
		},
	}

	// Set Keywords
	operatorCSV.Spec.Keywords = []string{
		"numa",
		"realtime",
		"cpu pinning",
		"hugepages",
	}

	// Set Provider
	operatorCSV.Spec.Provider = csvv1.AppLink{
		Name: "Red Hat",
	}

	// Set Description
	operatorCSV.Spec.Description = `
Performance Addon Operator provides the ability to enable advanced node performance tunings on a set of nodes.`

	operatorCSV.Spec.DisplayName = "Performance Addon Operator"

	if maintainers != nil {
		for name, email := range maintainers {
			operatorCSV.Spec.Maintainers = append(operatorCSV.Spec.Maintainers, csvv1.Maintainer{
				Name:  name,
				Email: email,
			})
		}
	}

	// Set Annotations
	if *skipRange != "" {
		operatorCSV.Annotations["olm.skipRange"] = *skipRange
	}

	// write CSV to out dir
	writer := strings.Builder{}
	csvtools.MarshallObject(operatorCSV, &writer)
	outputFilename := filepath.Join(*outputDir, finalizedCsvFilename())
	err = ioutil.WriteFile(outputFilename, []byte(writer.String()), 0644)
	if err != nil {
		panic(err)
	}

	fmt.Printf("CSV written to %s\n", outputFilename)
}

func readKeyValueMapFromFileOrPanic(filename string) map[string]string {
	kvMap := make(map[string]string)
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(data, &kvMap)
	if err != nil {
		panic(err)
	}
	return kvMap
}

func main() {
	flag.Parse()

	if *csvVersion == "" {
		log.Fatal("--csv-version is required")
	} else if *operatorCSVTemplate == "" {
		log.Fatal("--operator-csv-template-file is required")
	} else if *operatorImage == "" {
		log.Fatal("--operator-image is required")
	} else if *outputDir == "" {
		log.Fatal("--olm-bundle-directory is required")
	}

	var err error
	// Set correct csv versions and name
	semverVersion, err = semver.New(*csvVersion)
	if err != nil {
		panic(err)
	}

	extraAnnotations := make(map[string]string)
	if *annotationsFile != "" {
		extraAnnotations = readKeyValueMapFromFileOrPanic(*annotationsFile)
	}

	maintainers := make(map[string]string)
	if *maintainersFile != "" {
		maintainers = readKeyValueMapFromFileOrPanic(*maintainersFile)
	}

	// start with a fresh output directory if it already exists
	os.RemoveAll(*outputDir)

	// create output directory
	os.MkdirAll(*outputDir, os.FileMode(0775))

	generateUnifiedCSV(extraAnnotations, maintainers)
}
