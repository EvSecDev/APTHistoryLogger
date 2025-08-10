// APTHistoryLogger/m/v2
package main

import (
	"encoding/json"
	"reflect"
)

// Splits a big array into separate arrays, keeping other slices empty
func splitLargeArray(log LogJSON, fieldIndex int) (chunks []LogJSON, err error) {
	v := reflect.ValueOf(log)
	sliceVal := v.Field(fieldIndex)

	if sliceVal.Len() == 0 {
		return
	}

	tmp := log
	t := reflect.TypeOf(log)
	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).Type == reflect.TypeOf([]PackageInfo{}) && i != fieldIndex {
			reflect.ValueOf(&tmp).Elem().Field(i).Set(reflect.ValueOf([]PackageInfo{}))
		}
	}

	tmpB, err := json.Marshal(tmp)
	if err != nil {
		return
	}

	if len(tmpB) <= journalDMaxSize {
		// Small enough — keep in base log
		return
	}

	// Split in half and recurse
	mid := sliceVal.Len() / 2
	left := tmp
	right := tmp

	leftSlice := sliceVal.Slice(0, mid).Interface().([]PackageInfo)
	rightSlice := sliceVal.Slice(mid, sliceVal.Len()).Interface().([]PackageInfo)

	reflect.ValueOf(&left).Elem().Field(fieldIndex).Set(reflect.ValueOf(leftSlice))
	reflect.ValueOf(&right).Elem().Field(fieldIndex).Set(reflect.ValueOf(rightSlice))

	leftChunks, err := splitLargeArray(left, fieldIndex)
	if err != nil {
		return
	}
	rightChunks, err := splitLargeArray(right, fieldIndex)
	if err != nil {
		return
	}

	if leftChunks == nil {
		leftChunks = []LogJSON{left}
	}
	if rightChunks == nil {
		rightChunks = []LogJSON{right}
	}

	chunks = append(leftChunks, rightChunks...)
	return
}

// Splits package lists that exceed size as to fit in journald
// Non-package list fields are untouched and duplicated as many times as needed
func splitLog(log LogJSON) (chunks []LogJSON, err error) {
	baseLog := log

	t := reflect.TypeOf(log)
	var extraLogs []LogJSON

	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).Type == reflect.TypeOf([]PackageInfo{}) {
			var fieldChunks []LogJSON
			fieldChunks, err = splitLargeArray(log, i)
			if err != nil {
				return
			}
			if fieldChunks != nil {
				reflect.ValueOf(&baseLog).Elem().Field(i).Set(reflect.ValueOf([]PackageInfo{}))
				extraLogs = append(extraLogs, fieldChunks...)
			}
		}
	}

	chunks = append([]LogJSON{baseLog}, extraLogs...)
	return
}
