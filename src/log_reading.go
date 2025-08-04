// APTHistoryLogger/m/v2
package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"syscall"
)

func logReaderContinuous(logFileInput string, logFileOutput string) {
	if strings.HasSuffix(logFileInput, ".gz") {
		logError("Unsupported file input", fmt.Errorf("compressed files are not supported in continous mode"))
	}

	log, err := os.Open(logFileInput)
	logError("Failed to read log file", err)
	defer log.Close()

	logFileInode, logFileOffset, err := getLastPosition(logFileInput)
	logError("Failed to get position of last log read", err)

	_, err = log.Seek(logFileOffset, io.SeekStart)
	logError("Failed to resume in log", err)

	printMessage(verbosityDebug, "Starting log file read at offset %d\n", logFileOffset)

	// User requested output go to file
	var fileOutput *os.File
	if logFileOutput != "" {
		fileOutput, err = os.OpenFile(logFileOutput, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		logError("Failed to open output file", err)
		defer fileOutput.Close()
	}

	// Create background signal handler
	var signalBlocker sync.WaitGroup // Blocker so log reads/writes can finish before program exits
	go signalHandler(&signalBlocker, &logFileInode, &logFileOffset)

	// Create inotify background watcher
	fileHasChanged := make(chan bool, 1) // Main blocker for reading new lines
	fileHasRotated := make(chan bool, 1) // Notify when to switch file inodes and reset offset
	go changeWatcher(logFileInput, fileHasChanged, fileHasRotated)

	printMessage(verbosityProgress, "Starting log file watch\n")

	// Create a scanner to read the file
	scanner := bufio.NewScanner(log)

	if dryRunRequested {
		printMessage(verbosityStandard, "Dry-run requested, not processing log file. Exiting...\n")
		return
	}

	// Continous watching of the file
	var eventBlock string    // Buffer for the APT multi-line log entries
	var blockHasStarted bool // Flag to track if the current lines being prcessed are within a block
	for {
		// Process all available lines
		for scanner.Scan() {
			line := scanner.Text()

			logFileOffset, err = log.Seek(0, io.SeekCurrent)
			logError("Failed to get current offset while scanning", err)

			printMessage(verbosityDebug, "Read line, moved to new offset %d\n", logFileOffset)

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
				signalBlocker.Add(1)

				printMessage(verbosityProgress, "Parsing event fields\n")

				// Parse the log lines into single JSON
				var newLog LogJSON
				newLog, err = parseEvent(eventBlock)
				if err != nil {
					printMessage(verbosityNone, "Failed to parse log entry: %v: (%s)\n", err, strings.ReplaceAll(eventBlock, "\n", ":"))
				}

				jsonLine, err := json.Marshal(newLog)
				if err != nil {
					printMessage(verbosityNone, "Invalid JSON: %v: (%v)\n", err, newLog)
				}

				// Add newline after each JSON line
				jsonLine = append(jsonLine, '\n')

				// Output the formatted log
				if fileOutput != nil {
					fileOutput.Write(jsonLine)
				} else {
					fmt.Println(string(jsonLine))
				}

				// Save the end position of this block
				logFileOffset, err = log.Seek(0, io.SeekCurrent)
				logError("Failed to retrieve current position in log file", err)

				printMessage(verbosityDebug, "Processed log, currently at offset %d\n", logFileOffset)

				// Unblock signals after block finishes
				signalBlocker.Done()

				// Always ensure block buffer is empty at end of block
				eventBlock = ""
			}
		}

		// Check for errors in the scanner
		err = scanner.Err()
		logError("Error reading log", err)

		printMessage(verbosityDebug, "Currently at offset %d\n", logFileOffset)
		printMessage(verbosityProgress, "No more new lines, waiting for file changes\n")

		// Wait for inotify watcher to see that the log file has changed
		<-fileHasChanged

		select {
		case reopenLogFile := <-fileHasRotated:
			if reopenLogFile {
				// Reopen at file path
				log.Close()
				log, err = os.Open(logFileInput)
				logError("Failed to reopen rotated log file", err)

				// Retrieve new file inode
				fileInfo, err := os.Stat(logFileInput)
				logError("unable to stat new log file", err)
				stat := fileInfo.Sys().(*syscall.Stat_t)

				// Save new file inode to state var
				logFileInode = stat.Ino

				// Reset offset position for new file
				logFileOffset = 0
			}
		default:
			// No blocking if no rotation has occured
		}

		// Rescan for new lines after the last offset
		_, err = log.Seek(logFileOffset, io.SeekStart)
		logError("Failed to seek to last offset", err)

		printMessage(verbosityDebug, "Scanning for new lines at offset %d\n", logFileOffset)

		scanner = bufio.NewScanner(log) // Reset the scanner to continue scanning after the last offset
	}
}

func logReaderSearch(logFileInput string, searchParams SearchParameters) (parsedBuffer []LogJSON, err error) {
	log, err := os.Open(logFileInput)
	if err != nil {
		err = fmt.Errorf("failed to open log file: %v", err)
		return
	}
	defer log.Close()

	var logReader io.Reader

	if strings.HasSuffix(logFileInput, ".gz") {
		var gzReader *gzip.Reader
		gzReader, err = gzip.NewReader(log)
		if err != nil {
			err = fmt.Errorf("failed to open gz log file: %v", err)
			return
		}
		defer gzReader.Close()

		logReader = gzReader
	} else {
		logReader = log
	}

	scanner := bufio.NewScanner(logReader)

	var eventBlock string    // Buffer for the APT multi-line log entries
	var blockHasStarted bool // Flag to track if the current lines being prcessed are within a block
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

			printMessage(verbosityProgress, "Parsing event fields\n")

			// Parse the log lines into single JSON
			var newLog LogJSON
			newLog, err = parseEvent(eventBlock)
			if err != nil {
				err = fmt.Errorf("failed to parse log entry: %v", err)
				return
			}

			// Determine if this block matches any search criteria
			var searchMatched bool
			var matchedLog LogJSON
			searchMatched, matchedLog, err = newLog.findMatches(searchParams)
			if err != nil {
				err = fmt.Errorf("failed to search in log event: %v", err)
				return
			}

			if searchMatched {
				parsedBuffer = append(parsedBuffer, matchedLog)
			}

			// Always ensure block buffer is empty at end of block
			eventBlock = ""
		}
	}
	err = scanner.Err()
	if err != nil {
		err = fmt.Errorf("encountered error while reading log lines: %v", err)
		return
	}

	return
}
