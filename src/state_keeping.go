// APTHistoryLogger/m/v2
package main

import (
	"fmt"
	"os"
)

// Retrieve last read position for the log file from the state file
func getLastPosition() (position int64, err error) {
	_, err = os.Stat(stateDirectory)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(stateDirectory, 0750)
			if err != nil {
				err = fmt.Errorf("failed to create state file directory: %v", err)
				return
			}
		} else {
			err = fmt.Errorf("unable to access state directory: %v", err)
			return
		}
	}

	stateFile, err := os.OpenFile(logStateFilePath, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		if os.IsNotExist(err) {
			// If the file doesn't exist, start from the beginning
			position = 0
			return
		}
		return
	}
	defer stateFile.Close()

	_, err = fmt.Fscanf(stateFile, "%d", &position)
	if err != nil {
		if err.Error() == "EOF" {
			// If the file is empty, start from the beginning
			position = 0
			err = nil
			return
		}
		err = fmt.Errorf("unable to determine position: %v", err)
		return
	}

	return
}

// Save the current read position for the log file to the state file
func savePosition(position int64) (err error) {
	_, err = os.Stat(stateDirectory)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(stateDirectory, 0750)
			if err != nil {
				err = fmt.Errorf("failed to create state file directory: %v", err)
				return
			}
		} else {
			err = fmt.Errorf("unable to access state directory: %v", err)
			return
		}
	}

	stateFile, err := os.OpenFile(logStateFilePath, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return
	}
	defer stateFile.Close()

	_, err = fmt.Fprintf(stateFile, "%d", position)
	if err != nil {
		return
	}
	return
}
