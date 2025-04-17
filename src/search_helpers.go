// APTHistoryLogger/m/v2
package main

import (
	"fmt"
	"regexp"
	"time"
)

// I am sorry for this logic... but it do work
func (input LogJSON) findMatches(search SearchParameters) (searchMatched bool, matchedLogs LogJSON, err error) {
	if search.eventID != "" && search.eventID != input.EventID {
		return
	}

	startTimestamp, err := time.Parse(time.RFC3339, input.StartTimestamp)
	if err != nil {
		err = fmt.Errorf("failed parsing start time: %v", err)
		return
	}
	endTimestamp, err := time.Parse(time.RFC3339, input.EndTimeStamp)
	if err != nil {
		err = fmt.Errorf("failed parsing start time: %v", err)
		return
	}

	if startTimestamp.Before(search.startTimestamp) {
		return
	}
	if endTimestamp.After(search.endTimestamp) {
		return
	}
	matchedLogs.StartTimestamp = input.StartTimestamp
	matchedLogs.EndTimeStamp = input.EndTimeStamp

	if search.cmdLine != nil {
		if !search.cmdLine.MatchString(input.CommandLine) {
			return
		}
	}
	matchedLogs.CommandLine = input.CommandLine

	if search.userName != nil {
		if !search.userName.MatchString(input.RequestedBy) {
			return
		}
	}
	matchedLogs.RequestedBy = input.RequestedBy

	if search.userID != 0 {
		if search.userID != input.RequestedByUID {
			return
		}
	}
	matchedLogs.RequestedByUID = input.RequestedByUID

	matchedLogs.TotalPackages = input.TotalPackages
	matchedLogs.ElapsedSeconds = input.ElapsedSeconds

	if search.operation != nil {
		var operationMatchFound bool
		if input.InstallOperation && search.operation.MatchString("install") {
			matchedLogs.Install = input.Install
			matchedLogs.InstallOperation = input.InstallOperation

			operationMatchFound = true
		}
		if input.UpgradeOperation && search.operation.MatchString("upgrade") {
			matchedLogs.Upgrade = input.Upgrade
			matchedLogs.UpgradeOperation = input.UpgradeOperation

			operationMatchFound = true
		}
		if input.RemoveOperation && search.operation.MatchString("remove") {
			matchedLogs.Remove = input.Remove
			matchedLogs.RemoveOperation = input.RemoveOperation

			operationMatchFound = true
		}
		if input.PurgeOperation && search.operation.MatchString("purge") {
			matchedLogs.Purge = input.Purge
			matchedLogs.PurgeOperation = input.PurgeOperation

			operationMatchFound = true
		}
		if !operationMatchFound {
			return
		}
	} else if search.operation == nil {
		matchedLogs = input
	}

	if search.pkgName != nil || search.pkgVersion != nil {
		packageListMap := map[string][]PackageInfo{
			"install": matchedLogs.Install,
			"upgrade": matchedLogs.Upgrade,
			"remove":  matchedLogs.Remove,
			"purge":   matchedLogs.Purge,
		}

		var packageMatchesSearch bool
		for operationType, pkgList := range packageListMap {
			if len(pkgList) == 0 {
				continue
			}

			var matchedPackages []PackageInfo
			packageMatchesSearch, matchedPackages = searchForMatchingPackages(pkgList, search.pkgName, search.pkgVersion)
			if packageMatchesSearch {
				if operationType == "install" {
					matchedLogs.Install = matchedPackages
				} else if operationType == "upgrade" {
					matchedLogs.Upgrade = matchedPackages
				} else if operationType == "remove" {
					matchedLogs.Remove = matchedPackages
				} else if operationType == "purge" {
					matchedLogs.Purge = matchedPackages
				}
			}
		}
		if !packageMatchesSearch {
			return
		}
	}

	searchMatched = true
	return
}

func searchForMatchingPackages(packages []PackageInfo, nameRegex *regexp.Regexp, versionRegex *regexp.Regexp) (searchMatched bool, matchedPackages []PackageInfo) {
	for _, pkg := range packages {
		if nameRegex != nil {
			if nameRegex.MatchString(pkg.Name) {
				matchedPackages = append(matchedPackages, pkg)
				searchMatched = true
				continue
			}
		}

		if versionRegex != nil {
			if versionRegex.MatchString(pkg.Version) {
				searchMatched = true
				continue
			}
		}
	}

	return
}
