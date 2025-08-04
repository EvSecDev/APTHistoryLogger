// APTHistoryLogger/m/v2
package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Separate thread to listen for signals and ensure cleanup prior to exit
func signalHandler(signalBlocker *sync.WaitGroup, fileInode *uint64, fileOffsetPosition *int64) {
	printMessage(verbosityDebug, "Starting signal handling thread\n")

	// Channel for handling interrupt signals (to ensure we save the position on exit)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	sig := <-sigChan

	printMessage(verbosityStandard, "Received signal: %v\n", sig)

	// Wait for current block parsing to complete before exiting
	signalBlocker.Wait()

	// Save the current file position before exiting
	printMessage(verbosityData, "Saving current log file inode (%d) and position (%d)\n", *fileInode, *fileOffsetPosition)
	savePosition(*fileInode, *fileOffsetPosition)

	printMessage(verbosityStandard, "Shutting down\n")
	os.Exit(0)
}

func changeWatcher(logFileInput string, fileHasChanged chan bool, fileHasRotated chan bool) {
	printMessage(verbosityProgress, "Starting inotify thread to watch for file/directory changes\n")

	// Open the inotify instance
	fd, err := syscall.InotifyInit()
	logError("Failed to initialize inotify", err)
	defer syscall.Close(fd)

	// Track active watcher fd's for dynamic cleanup
	watchDescriptors := make(map[string]int)
	defer func() {
		for descName, descriptor := range watchDescriptors {
			printMessage(verbosityDebug, "Cleaning up Inotify %s descriptor %d", descName, descriptor)
			syscall.InotifyRmWatch(fd, uint32(descriptor))
		}
	}()

	// Add watcher for the log file
	watchDescriptorFile, err := syscall.InotifyAddWatch(fd, logFileInput, syscall.IN_MODIFY|syscall.IN_CLOSE_WRITE)
	logError("Failed to add log file to inotify watcher", err)
	watchDescriptors["file"] = watchDescriptorFile

	// Add watcher for the log dir
	logDirectory := filepath.Dir(logFileInput)
	watchDescriptorDir, err := syscall.InotifyAddWatch(fd, logDirectory, syscall.IN_MOVED_FROM|syscall.IN_MOVED_TO|syscall.IN_DELETE|syscall.IN_CREATE)
	logError("Failed to add directory to inotify watcher", err)
	watchDescriptors["dir"] = watchDescriptorDir

	// Create a buffer to read the events
	buf := make([]byte, syscall.SizeofInotifyEvent+8192)
	logFileName := filepath.Base(logFileInput)

	for {
		// Read the event
		n, err := syscall.Read(fd, buf)
		logError("Error reading inotify event", err)

		var offset uint32
		for offset <= uint32(n)-syscall.SizeofInotifyEvent {
			var event syscall.InotifyEvent

			// Retrieve the event
			eventBytes := buf[offset : offset+syscall.SizeofInotifyEvent]
			reader := bytes.NewReader(eventBytes)
			err = binary.Read(reader, binary.LittleEndian, &event)
			logError("Failed to read event content", err)

			// Name field has the filename for dir events (null-terminated)
			nameBytes := buf[offset+syscall.SizeofInotifyEvent : offset+syscall.SizeofInotifyEvent+uint32(event.Len)]
			name := string(nameBytes)
			name = strings.TrimRight(name, "\x00")

			// File modified
			if event.Mask&syscall.IN_MODIFY != 0 && event.Wd == int32(watchDescriptorFile) {
				printMessage(verbosityProgress, "File modified: %s\n", logFileInput)
				fileHasChanged <- true
			}

			// Directory events - only look for our file
			if event.Wd == int32(watchDescriptorDir) && name == logFileName {
				if (event.Mask & (syscall.IN_MOVED_FROM | syscall.IN_MOVED_TO | syscall.IN_DELETE | syscall.IN_CREATE)) != 0 {
					printMessage(verbosityProgress, "Log file rotated: %s\n", logFileInput)
					// Ensure new file is created before adding watcher for new inode
					for {
						if _, err := os.Stat(logFileInput); err == nil {
							break
						}
						time.Sleep(100 * time.Millisecond)
					}

					// Cleanup watcher for old inode
					syscall.InotifyRmWatch(fd, uint32(watchDescriptorFile))

					// Add watcher for new inode
					watchDescriptorFile, err = syscall.InotifyAddWatch(fd, logFileInput, syscall.IN_MODIFY|syscall.IN_CLOSE_WRITE)
					logError("Failed to add rotated log file to inotify watcher", err)
					watchDescriptors["file"] = watchDescriptorFile

					fileHasRotated <- true // send value to buffer so its available after main thread is unblocked
					fileHasChanged <- true // unblock main thread
				}
			}

			// Move the offset forward to the next event
			offset += syscall.SizeofInotifyEvent + uint32(event.Len)
		}
	}
}
