// APTHistoryLogger/m/v2
package main

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"slices"
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
		case "Reinstall":
			newLog.Reinstall, err = parsePackages(fieldValue)
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
	}

	if len(newLog.Install) > 0 {
		newLog.InstallOperation = true
	}
	if len(newLog.Reinstall) > 0 {
		newLog.ReinstallOperation = true
	}
	if len(newLog.Upgrade) > 0 {
		newLog.UpgradeOperation = true
	}
	if len(newLog.Remove) > 0 {
		newLog.RemoveOperation = true
	}
	if len(newLog.Purge) > 0 {
		newLog.PurgeOperation = true
	}

	// Use raw string of log data structure as source of event ID
	eventBytes := fmt.Appendf(nil, "%v", newLog)
	newLog.EventID = generateUUID(eventBytes)

	// Calculate elapsed time of apt operation
	newLog.ElapsedSeconds, err = calculateElaspedTime(newLog.StartTimestamp, newLog.EndTimeStamp)
	if err != nil {
		err = fmt.Errorf("failed to calculate elapsed time: %v", err)
		return
	}

	// Add total package number for this operation
	newLog.TotalPackages = len(newLog.Install) + len(newLog.Reinstall) + len(newLog.Upgrade) + len(newLog.Remove) + len(newLog.Purge)

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

		if len(pkgFields) < 2 {
			err = fmt.Errorf("could identify more than 2 fields to extract name")
			return
		}

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
		} else if len(pkgFields) == 3 {
			packageInfo.Version = pkgFields[2]
		}

		// Add to main list
		packageList = append(packageList, packageInfo)
	}

	return
}

func (opts SearchOptions) parseSearchOptions() (validatedOpts SearchParameters, err error) {
	const searchTimestampLayout string = "2006-01-02T15:04:05"

	if opts.eventID != "" {
		validatedOpts.eventID = opts.eventID
	}

	if opts.startTimestamp != "" {
		validatedOpts.startTimestamp, err = time.Parse(searchTimestampLayout, opts.startTimestamp)
		if err != nil {
			err = fmt.Errorf("failed parsing start time: %v", err)
			return
		}
	} else if opts.startTimestamp == "" {
		currentTime := time.Now()
		oneWeekAgo := currentTime.AddDate(0, 0, -7)
		validatedOpts.startTimestamp = oneWeekAgo
	}

	if opts.endTimestamp != "" {
		validatedOpts.endTimestamp, err = time.Parse(searchTimestampLayout, opts.endTimestamp)
		if err != nil {
			err = fmt.Errorf("failed parsing end time: %v", err)
			return
		}
	} else if opts.endTimestamp == "" {
		currentTime := time.Now()
		validatedOpts.endTimestamp = currentTime
	}

	if opts.cmdLine != "" {
		validatedOpts.cmdLine, err = regexp.Compile(opts.cmdLine)
		if err != nil {
			err = fmt.Errorf("failed to compile command line text as regex: %v", err)
			return
		}
	}
	if opts.pkgName != "" {
		validatedOpts.pkgName, err = regexp.Compile(opts.pkgName)
		if err != nil {
			err = fmt.Errorf("failed to compile package name as regex: %v", err)
			return
		}
	}
	if opts.pkgVersion != "" {
		validatedOpts.pkgVersion, err = regexp.Compile(opts.pkgVersion)
		if err != nil {
			err = fmt.Errorf("failed to compile package version as regex: %v", err)
			return
		}
	}
	if opts.operation != "" {
		opts.operation = strings.ToLower(opts.operation)

		operationCheckRegex := regexp.MustCompile(`^(install|reinstall|upgrade|remove|purge)(\|(install|reinstall|upgrade|remove|purge))*$`)

		if !operationCheckRegex.MatchString(opts.operation) {
			err = fmt.Errorf("invalid operation type: must be install, reinstall, upgrade, remove, or purge (separated by '|' optionally)")
			return
		}

		validatedOpts.operation, err = regexp.Compile(opts.operation)
		if err != nil {
			err = fmt.Errorf("failed to compile operation as regex: %v", err)
			return
		}
	}
	if opts.userName != "" {
		validatedOpts.userName, err = regexp.Compile(opts.userName)
		if err != nil {
			err = fmt.Errorf("failed to compile user name as regex: %v", err)
			return
		}
	}
	if opts.userID != "" {
		validatedOpts.userID, err = strconv.Atoi(opts.userID)
		if err != nil {
			err = fmt.Errorf("failed to compile user ID as number: %v", err)
			return
		}
	}

	return
}

func sortLogsByTimestamp(logs []LogJSON, order string) (sortedLogs []LogJSON) {
	sortedLogs = logs
	layout := time.RFC3339

	slices.SortFunc(sortedLogs, func(a, b LogJSON) int {
		t1, err1 := time.Parse(layout, a.StartTimestamp)
		t2, err2 := time.Parse(layout, b.StartTimestamp)

		if err1 != nil || err2 != nil {
			if a.StartTimestamp < b.StartTimestamp {
				return -1
			} else if a.StartTimestamp > b.StartTimestamp {
				return 1
			}
			return 0
		}

		if order == "asc" {
			return t1.Compare(t2)
		}
		return t2.Compare(t1)
	})

	return
}

func generateUUID(inputData []byte) (uuid string) {
	// Hash the data
	hasher := sha256.New()
	hasher.Write(inputData)
	hashBytes := hasher.Sum(nil)

	// Only using first 16 bytes
	uuidBytes := hashBytes[:16]

	// Convert to UUID format
	uuid = fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuidBytes[0:4],
		uuidBytes[4:6],
		uuidBytes[6:8],
		uuidBytes[8:10],
		uuidBytes[10:16])

	return
}
