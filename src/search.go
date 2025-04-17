// APTHistoryLogger/m/v2
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func search(inputPath string, userSearchOpts SearchOptions) {
	searchParams, err := userSearchOpts.parseSearchOptions()
	logError("Invalid search parameter", err)

	// Error is irrelevant, actual file access errors are addressed in log search function
	logMeta, _ := os.Stat(inputPath)

	var searchFiles []string
	if logMeta == nil {
		searchFiles, err = filepath.Glob(inputPath)
		logError("Failed to read input file choice", err)
	} else if logMeta.Mode().IsRegular() {
		searchFiles = append(searchFiles, inputPath)
	} else if logMeta.Mode().IsDir() {
		var logFiles []os.DirEntry
		logFiles, err := os.ReadDir(inputPath)
		logError("Failed to read directory contents", err)

		for _, logFile := range logFiles {
			if !logFile.IsDir() {
				absLogFile := filepath.Join(inputPath, logFile.Name())
				searchFiles = append(searchFiles, absLogFile)
			}
		}
	}

	var rawSearchResults []LogJSON

	for _, searchFile := range searchFiles {
		matchedEntries, err := logReaderSearch(searchFile, searchParams)
		logError("Failed to search log", err)

		rawSearchResults = append(rawSearchResults, matchedEntries...)
	}

	if len(rawSearchResults) == 0 {
		printMessage(verbosityStandard, "Search returned no results\n")
		return
	}

	sortedSearchResults := sortLogsByTimestamp(rawSearchResults, userSearchOpts.outputOrder)

	var outputJSON SearchOutput
	outputJSON.TotalResults = len(sortedSearchResults)
	outputJSON.Results = sortedSearchResults

	searchResults, err := json.MarshalIndent(outputJSON, "", "  ")
	logError("Invalid JSON", err)

	printMessage(verbosityStandard, "%s\n", string(searchResults))
}
