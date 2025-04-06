// APTHistoryLogger/m/v2
package main

import (
	"fmt"
	"os"
	"time"
)

// ###################################
//      GLOBAL HELPERS
// ###################################

// Print message to stdout
// Message will only print if the global verbosity level is equal to or smaller than requiredVerbosityLevel
func printMessage(requiredVerbosityLevel int, message string, vars ...interface{}) {
	// No output for verbosity level 0
	if globalVerbosityLevel == 0 {
		return
	}

	// Add timestamps to verbosity levels 2 and up (but only when the timestamp will get printed)
	if globalVerbosityLevel >= 2 && requiredVerbosityLevel <= globalVerbosityLevel {
		currentTime := time.Now()
		timestamp := currentTime.Format("15:04:05.000000")
		message = timestamp + ": " + message
	}

	// Required stdout message verbosity level is equal to or less than global verbosity level
	if requiredVerbosityLevel <= globalVerbosityLevel {
		fmt.Printf(message, vars...)
	}
}

func logError(errorDescription string, errorMessage error) {
	// return early if no error to process
	if errorMessage == nil {
		return
	}

	// Print the error
	fmt.Fprintf(os.Stderr, "%s: %v\n", errorDescription, errorMessage)
	os.Exit(1)
}
