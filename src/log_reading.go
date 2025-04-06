// APTHistoryLogger/m/v2
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

func logReaderContinuous(logFileInput string, logRefreshSeconds int) {
	log, err := os.Open(logFileInput)
	logError("Failed to read log file", err)

	position, err := getLastPosition()
	logError("Failed to get position of last log read", err)

	_, err = log.Seek(position, io.SeekStart)
	logError("Failed to resume in log", err)

	// WaitGroup to ensure that parsing finishes before handling signals
	var wg sync.WaitGroup

	// Channel for handling interrupt signals (to ensure we save the position on exit)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Start a goroutine to listen for exit signals and save the position
	go func() {
		sig := <-sigChan

		fmt.Printf("Shutting down, received signal: %v\n", sig)

		// Wait for current block parsing to complete before exiting
		wg.Wait()

		// Save the current file position before exiting
		savePosition(position)
		os.Exit(0)
	}()

	// Wait time for log rescan
	logRescanWaitTime := time.Duration(logRefreshSeconds) * time.Second

	// Buffer for the APT multi-line log entries
	var eventBlock string

	// Flag to track if the current lines being prcessed are within a block
	var blockHasStarted bool

	// Create a scanner to read the file
	var scanner *bufio.Scanner
	scanner = bufio.NewScanner(log)

	// Continous watching of the file
	for {
		// Process all available lines
		for scanner.Scan() {
			line := scanner.Text()

			if strings.HasPrefix(line, "Start-Date: ") {
				// Always ensure block buffer is empty on new block
				eventBlock = ""

				// Marks all lines after this one as "to be added to block"
				blockHasStarted = true
			}

			// Add all lines within the block to the buffer
			if blockHasStarted {
				eventBlock += line + "\n"
			}

			// Once at the end of block, parse the entries
			if strings.HasPrefix(line, "End-Date: ") {
				blockHasStarted = false

				// Block signals while parsing block
				wg.Add(1)

				var newLog LogJSON
				newLog, err = parseEvent(eventBlock)
				logError("Parsed to parse log entry", err)

				jsonLine, err := json.Marshal(newLog)
				logError("Invalid json", err)

				fmt.Printf("%v\n", string(jsonLine))

				// Save the end position of this block
				position, _ = log.Seek(0, io.SeekCurrent)

				// Unblock signals after block finishes
				wg.Done()

				// Always ensure block buffer is empty at end of block
				eventBlock = ""
			}
		}

		// Check for errors in the scanner
		if err := scanner.Err(); err != nil {
			logError("Error reading log", err)
		}

		// Get the current file offset
		offset, err := log.Seek(0, io.SeekCurrent)
		if err != nil {
			fmt.Println("Error getting current file offset:", err)
			return
		}

		// Wait for a period before checking again
		time.Sleep(logRescanWaitTime)

		// Rescan for new lines after the last offset
		log.Seek(offset, io.SeekStart)
		scanner = bufio.NewScanner(log) // Reset the scanner to continue scanning after the last offset
	}
}
