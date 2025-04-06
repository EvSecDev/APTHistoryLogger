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
	"unsafe"
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
				event := (*syscall.InotifyEvent)(unsafe.Pointer(&buf[offset]))

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
	scanner = bufio.NewScanner(log)

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
				logError("Failed to parse log entry", err)

				jsonLine, err := json.Marshal(newLog)
				logError("Invalid JSON", err)

				// Add newline after each JSON line
				jsonLine = append(jsonLine, '\n')

				// Output the formatted log
				if fileOutput != nil {
					fileOutput.Write(jsonLine)
				} else {
					fmt.Println(jsonLine)
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
		if err := scanner.Err(); err != nil {
			logError("Error reading log", err)
		}

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
