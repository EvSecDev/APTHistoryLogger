// APTHistoryLogger/m/v2
package main

import (
	"reflect"
	"testing"
)

func TestParsePackages(t *testing.T) {
	tests := []struct {
		name        string
		rawList     string
		want        []PackageInfo
		expectError bool
	}{
		{
			name:    "Standard Automatic",
			rawList: "libc-ares2:amd64 (1.18.1-3, automatic), linux-image-6.1.0-37-amd64:amd64 (6.1.140-1, automatic)",
			want: []PackageInfo{
				{Name: "libc-ares2", Arch: "amd64", Version: "1.18.1-3"},
				{Name: "linux-image-6.1.0-37-amd64", Arch: "amd64", Version: "6.1.140-1", OldVersion: ""},
			},
			expectError: false,
		},
		{
			name:    "Standard Regular Upgrade",
			rawList: "libperl5.36:amd64 (5.36.0-7+deb12u1, 5.36.0-7+deb12u2), libgomp1:amd64 (12.2.0-14, 12.2.0-14+deb12u1)",
			want: []PackageInfo{
				{Name: "libperl5.36", Arch: "amd64", Version: "5.36.0-7+deb12u2", OldVersion: "5.36.0-7+deb12u1"},
				{Name: "libgomp1", Arch: "amd64", Version: "12.2.0-14+deb12u1", OldVersion: "12.2.0-14"},
			},
			expectError: false,
		},
		{
			name:    "Standard Regular Install",
			rawList: "libxt6:amd64 (1:1.2.1-1.1), libluajit2-5.1-common:amd64 (2.1-20230119-1)",
			want: []PackageInfo{
				{Name: "libxt6", Arch: "amd64", Version: "1:1.2.1-1.1"},
				{Name: "libluajit2-5.1-common", Arch: "amd64", Version: "2.1-20230119-1"},
			},
			expectError: false,
		},
		{
			name:        "Error with less than 2 fields",
			rawList:     "pkgnover ()",
			want:        nil,
			expectError: true,
		},
		{
			name:        "Empty input",
			rawList:     "",
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parsePackages(test.rawList)
			if (err != nil) != test.expectError {
				t.Errorf("parsePackages() error = %v, expectError %v", err, test.expectError)
				return
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("parsePackages() = %v, want %v", got, test.want)
			}
		})
	}
}
