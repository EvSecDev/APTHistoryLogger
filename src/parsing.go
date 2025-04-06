// APTHistoryLogger/m/v2
package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func parseEvent(event string) (newLog LogJSON, err error) {
	eventFields := strings.Split(event, "\n")

	// Attempt to parse each field in event
	for _, eventField := range eventFields {
		// Skip empty
		if eventField == "" {
			continue
		}

		field := strings.Split(eventField, ": ")
		if len(field) != 2 {
			err = fmt.Errorf("unable to parse event field: unexpected value='%s'", eventField)
			return
		}

		// Separate Field prefix (name) from its value
		fieldPrefix := field[0]
		fieldValue := field[1]

		// Parse field values based on known prefixes
		switch fieldPrefix {
		case "Start-Date":
			newLog.StartTimestamp, err = parseTimestamp(fieldValue)
		case "End-Date":
			newLog.EndTimeStamp, err = parseTimestamp(fieldValue)
		case "Commandline":
			newLog.CommandLine = fieldValue
		case "Requested-By":
			newLog.RequestedBy, newLog.RequestedByUID, err = parseRequester(fieldValue)
		case "Error":
			newLog.Error = fieldValue
		case "Install":
			newLog.Install, err = parsePackages(fieldValue)
		case "Upgrade":
			newLog.Upgrade, err = parsePackages(fieldValue)
		case "Remove":
			newLog.Remove, err = parsePackages(fieldValue)
		case "Purge":
			newLog.Purge, err = parsePackages(fieldValue)
		default:
			err = fmt.Errorf("unknown prefix '%s' with value '%s'", fieldPrefix, fieldValue)
		}

		// Check any errors after parsing
		if err != nil {
			err = fmt.Errorf("failed to parse field '%s': %v", fieldPrefix, err)
			return
		}

		// Calculate elapsed time of apt operation
		newLog.ElapsedSeconds, err = calculateElaspedTime(newLog.StartTimestamp, newLog.EndTimeStamp)
		if err != nil {
			err = fmt.Errorf("failed to calculate elapsed time: %v", err)
			return
		}

		// Add total package number for this operation
		newLog.TotalPackages = len(newLog.Install) + len(newLog.Upgrade) + len(newLog.Remove) + len(newLog.Purge)
	}
	return
}

func parseTimestamp(rawTimestamp string) (timestamp string, err error) {
	layout := "2006-01-02  15:04:05"
	dateTime, err := time.ParseInLocation(layout, rawTimestamp, time.Local)
	if err != nil {
		err = fmt.Errorf("failed parsing timestamp: %v", err)
		return
	}
	timestamp = dateTime.Format(time.RFC3339)

	return
}

func calculateElaspedTime(startTime string, endTime string) (elapsedSeconds int, err error) {
	// Assume input is ISO8601 format (RFC3339)
	layout := time.RFC3339

	// Parse the start time
	start, err := time.Parse(layout, startTime)
	if err != nil {
		err = fmt.Errorf("invalid start time: %v", err)
		return
	}

	// Parse the end time
	end, err := time.Parse(layout, endTime)
	if err != nil {
		err = fmt.Errorf("invalid end time: %v", err)
		return
	}

	// Calculate the duration between the start and end times
	duration := end.Sub(start)

	// Return the duration in whole seconds
	elapsedSeconds = int(duration.Seconds())
	return
}

func parseRequester(user string) (requester string, requeterUID int, err error) {
	userInfo := strings.Split(user, " ")
	if len(userInfo) == 0 {
		err = fmt.Errorf("invalid length (length 0)")
		return
	}

	requester = userInfo[0]

	if len(userInfo) == 2 {
		userInfo[1] = strings.TrimPrefix(userInfo[1], "(")
		userInfo[1] = strings.TrimSuffix(userInfo[1], ")")

		requeterUID, err = strconv.Atoi(userInfo[1])
		if err != nil {
			err = fmt.Errorf("failed to convert UID string '%s' to int", userInfo[1])
			return
		}
	}

	return
}

func parsePackages(rawList string) (packageList []PackageInfo, err error) {
	// Split each package info on unique separator
	fullList := strings.Split(rawList, "), ")

	// Process each package in list for each specific info
	for _, pkg := range fullList {
		var packageInfo PackageInfo

		// Remove known separators to get space-separated list
		pkg = strings.TrimSuffix(pkg, ")")
		pkg = strings.Replace(pkg, "(", "", 1)
		pkg = strings.Replace(pkg, ",", "", 1)
		pkg = strings.Replace(pkg, ":", " ", 1)

		// Split on spaces
		pkgFields := strings.Fields(pkg)

		// Extract fields
		packageInfo.Name = pkgFields[0]
		packageInfo.Arch = pkgFields[1]
		if len(pkgFields) == 4 {
			// Automatic is from installs - installs do not require OldVersion
			if pkgFields[3] == "automatic" {
				packageInfo.Version = pkgFields[2]
			} else {
				packageInfo.OldVersion = pkgFields[2]
				packageInfo.Version = pkgFields[3]
			}
		} else {
			packageInfo.Version = pkgFields[2]
		}

		// Add to main list
		packageList = append(packageList, packageInfo)
	}

	return
}
