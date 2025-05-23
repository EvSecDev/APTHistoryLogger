// APTHistoryLogger/m/v2
package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

func logReaderContinuous(logFileInput string, logFileOutput string) {
	log, err := os.Open(logFileInput)
	logError("Failed to read log file", err)
	defer log.Close()

	position, err := getLastPosition()
	logError("Failed to get position of last log read", err)

	_, err = log.Seek(position, io.SeekStart)
	logError("Failed to resume in log", err)

	// User requested output go to file
	var fileOutput *os.File
	if logFileOutput != "" {
		fileOutput, err = os.OpenFile(logFileOutput, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		logError("Failed to open output file", err)
		defer fileOutput.Close()
	}

	var logReader io.Reader

	if strings.HasSuffix(logFileInput, ".gz") {
		gzReader, err := gzip.NewReader(log)
		logError("Failed to read gzip'ed log file", err)
		defer gzReader.Close()

		logReader = gzReader
	} else {
		logReader = log
	}

	// WaitGroup to ensure that parsing finishes before program exits
	var signalBlocker sync.WaitGroup

	// Channel for handling interrupt signals (to ensure we save the position on exit)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Separate thread to listen for signals and ensure cleanup prior to exit
	go func() {
		sig := <-sigChan

		printMessage(verbosityStandard, "Received signal: %v\n", sig)

		// Wait for current block parsing to complete before exiting
		signalBlocker.Wait()

		// Save the current file position before exiting
		printMessage(verbosityData, "Saving current log file position (%v)\n", position)
		savePosition(position)

		printMessage(verbosityStandard, "Shutting down\n")
		os.Exit(0)
	}()

	printMessage(verbosityProgress, "Setting up inotify to watch for log file changes\n")

	// Open the inotify instance
	fd, err := syscall.InotifyInit()
	logError("Failed to initialize inotify", err)
	defer syscall.Close(fd)

	// Add watcher for the log file
	watchDescriptor, err := syscall.InotifyAddWatch(fd, logFileInput, syscall.IN_MODIFY|syscall.IN_CLOSE_WRITE)
	logError("Failed to add log file to inotify watcher", err)
	defer syscall.InotifyRmWatch(fd, uint32(watchDescriptor))

	// Create a buffer to read the events
	buf := make([]byte, syscall.SizeofInotifyEvent+1024)

	// Create a channel to signal when to continue to the next iteration
	done := make(chan bool)

	// Start the goroutine to read and handle events
	go func() {
		for {
			// Read the event
			n, err := syscall.Read(fd, buf)
			logError("Error reading inotify event", err)

			// Parse the event
			var offset uint32
			for offset <= uint32(n)-syscall.SizeofInotifyEvent {
				var event syscall.InotifyEvent

				// Retrieve the event
				eventBytes := buf[offset : offset+syscall.SizeofInotifyEvent]
				reader := bytes.NewReader(eventBytes)
				err = binary.Read(reader, binary.LittleEndian, &event)
				logError("Failed to read event content", err)

				if event.Mask&syscall.IN_MODIFY != 0 {
					printMessage(verbosityProgress, "File modified: %s\n", logFileInput)
				}

				// Move the offset forward to the next event
				offset += syscall.SizeofInotifyEvent + uint32(event.Len)
			}

			// Signal that we are done processing the event
			done <- true
		}
	}()

	printMessage(verbosityProgress, "Starting log file watch\n")

	// Create a scanner to read the file
	var scanner *bufio.Scanner
	scanner = bufio.NewScanner(logReader)

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
				position, err = log.Seek(0, io.SeekCurrent)
				logError("Failed to retrieve current position in log file", err)

				// Unblock signals after block finishes
				signalBlocker.Done()

				// Always ensure block buffer is empty at end of block
				eventBlock = ""
			}
		}

		// Check for errors in the scanner
		err = scanner.Err()
		logError("Error reading log", err)

		// Get the current file offset
		offset, err := log.Seek(0, io.SeekCurrent)
		if err != nil {
			fmt.Println("Error getting current file offset:", err)
			return
		}

		printMessage(verbosityProgress, "No more new lines, waiting for file changes\n")

		// Wait for inotify watcher to see that the log file has changed
		<-done

		// Rescan for new lines after the last offset
		log.Seek(offset, io.SeekStart)
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
