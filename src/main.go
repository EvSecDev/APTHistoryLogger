// APTHistoryLogger/m/v2
package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"time"
)

// ###################################
//	GLOBAL CONSTANTS
// ###################################

const (
	stateDirectory   string = "/var/lib/APTHistoryLogger"
	logStateFilePath string = "/var/lib/APTHistoryLogger/log.state"
	journalDMaxSize         = 16 * 999 // Try to stay well below journald max log entry
)
const ( // Descriptive Names for available verbosity levels
	verbosityNone int = iota
	verbosityStandard
	verbosityProgress
	verbosityData
	verbosityFullData
	verbosityDebug
)

// ###################################
//  GLOBAL VARIABLES
// ###################################

type LogJSON struct {
	EventID            string        `json:"EventID"`
	CommandLine        string        `json:"CommandLine"`
	StartTimestamp     string        `json:"StartTimestamp"`
	EndTimeStamp       string        `json:"EndTimeStamp"`
	ElapsedSeconds     int           `json:"ElapsedSeconds"`
	RequestedBy        string        `json:"RequestedBy,omitempty"`
	RequestedByUID     int           `json:"RequestedByUID,omitempty"`
	TotalPackages      int           `json:"TotalPackages,omitempty"`
	Install            []PackageInfo `json:"Install,omitempty"`
	Reinstall          []PackageInfo `json:"Reinstall,omitempty"`
	Upgrade            []PackageInfo `json:"Upgrade,omitempty"`
	Remove             []PackageInfo `json:"Remove,omitempty"`
	Purge              []PackageInfo `json:"Purge,omitempty"`
	InstallOperation   bool          `json:"InstallOperation,omitempty"`
	ReinstallOperation bool          `json:"ReinstallOperation,omitempty"`
	UpgradeOperation   bool          `json:"UpgradeOperation,omitempty"`
	RemoveOperation    bool          `json:"RemoveOperation,omitempty"`
	PurgeOperation     bool          `json:"PurgeOperation,omitempty"`
	Error              string        `json:"Error,omitempty"`
}

type PackageInfo struct {
	Name       string `json:"package"`
	Arch       string `json:"archiecture"`
	OldVersion string `json:"oldversion,omitempty"`
	Version    string `json:"version"`
}

// User chosen search parameters
type SearchOptions struct {
	eventID        string
	outputOrder    string
	startTimestamp string
	endTimestamp   string
	pkgName        string
	pkgVersion     string
	operation      string
	cmdLine        string
	userName       string
	userID         string
}

// Parsed search parameters
type SearchParameters struct {
	eventID        string
	startTimestamp time.Time
	endTimestamp   time.Time
	pkgName        *regexp.Regexp
	pkgVersion     *regexp.Regexp
	operation      *regexp.Regexp
	cmdLine        *regexp.Regexp
	userName       *regexp.Regexp
	userID         int
}

type SearchOutput struct {
	TotalResults int       `json:"totalresults"`
	Results      []LogJSON `json:"results"`
}

// #### Written to only from main

var dryRunRequested bool // for printing relevant information and bailing out before processing

// Integer for printing increasingly detailed information as program progresses
//
//	0 - None: quiet (prints nothing but errors)
//	1 - Standard: normal progress messages
//	2 - Progress: more progress messages (no actual data outputted)
//	3 - Data: shows limited data being processed
//	4 - FullData: shows full data being processed
//	5 - Debug: shows extra data during processing (raw bytes)
var globalVerbosityLevel int

// ###################################
//      MAIN - START
// ###################################

func main() {
	// Program Argument Variables
	var daemonMode bool
	var logFileInput string
	var outputFile string
	var searchMode bool
	var searchOpts SearchOptions
	var versionInfoRequested bool
	var versionRequested bool

	const usage = `
APT History Logger (APTHL)
  Watches apt history.log and parses events into JSON

  Options:
    -d, --daemon                                   Run continously
    -l, --log-file <path/to/log>                   Input log file [default: /var/log/apt/history.log]
    -o, --out-file <path/to/file>                  Output to a file instead of stdout
    -s, --search                                   Search through log file for given search parameters
        --time-order      <asc|desc>               Order search output ascending/descending by start timestamp [default: asc]
        --start-timestamp <2010-12-31T23:59:59>    Filter start time of search [default: 1 week ago]
        --end-timestamp   <2011-12-31T23:59:59>    Filter end time of search [default: now]
        --event-id        <uuid>                   Filter by specific event id
        --command-line    <text>                   Filter command line
        --package-name    <pkg>                    Filter package name
        --package-version <ver>                    Filter package version
        --operation <op>                           Filter APT operation (install|reinstall|upgrade|remove|purge)
        --user-name <name>                         Filter user that initiated operation by name
        --user-uid  <num>                          Filter user that initiated operation by ID
    -T, --dry-run                                  Does all startups except process the log file
    -h, --help                                     Show this help menu
    -v, --verbose <0...5>                          Increase details and frequency of progress messages [default: 1]
    -V, --version                                  Show version and packages
        --versionid                                Show only version number

Report bugs to: dev@evsec.net
APTHistorLogger home page: <https://github.com/EvSecDev/APTHistoryLogger>
General help using GNU software: <https://www.gnu.org/gethelp/>
`
	// Read Program Arguments
	flag.BoolVar(&daemonMode, "d", false, "")
	flag.BoolVar(&daemonMode, "daemon", false, "")
	flag.StringVar(&logFileInput, "l", "/var/log/apt/history.log", "")
	flag.StringVar(&logFileInput, "log-file", "/var/log/apt/history.log", "")
	flag.StringVar(&outputFile, "o", "", "")
	flag.StringVar(&outputFile, "out-file", "", "")
	flag.BoolVar(&searchMode, "s", false, "")
	flag.BoolVar(&searchMode, "search", false, "")
	flag.StringVar(&searchOpts.outputOrder, "time-order", "asc", "")
	flag.StringVar(&searchOpts.startTimestamp, "start-timestamp", "", "")
	flag.StringVar(&searchOpts.endTimestamp, "end-timestamp", "", "")
	flag.StringVar(&searchOpts.eventID, "event-id", "", "")
	flag.StringVar(&searchOpts.cmdLine, "command-line", "", "")
	flag.StringVar(&searchOpts.pkgName, "package-name", "", "")
	flag.StringVar(&searchOpts.pkgVersion, "package-version", "", "")
	flag.StringVar(&searchOpts.operation, "operation", "", "")
	flag.StringVar(&searchOpts.userName, "user-name", "", "")
	flag.StringVar(&searchOpts.userID, "user-uid", "", "")
	flag.BoolVar(&dryRunRequested, "T", false, "")
	flag.BoolVar(&dryRunRequested, "dry-run", false, "")
	flag.IntVar(&globalVerbosityLevel, "v", 1, "")
	flag.IntVar(&globalVerbosityLevel, "verbosity", 1, "")
	flag.BoolVar(&versionInfoRequested, "V", false, "")
	flag.BoolVar(&versionInfoRequested, "version", false, "")
	flag.BoolVar(&versionRequested, "versionid", false, "")

	flag.Usage = func() { fmt.Printf("Usage: %s [OPTIONS]...%s", os.Args[0], usage) }
	flag.Parse()

	const progVersion string = "v1.0.0"
	if versionInfoRequested {
		fmt.Printf("APTHistoryLogger %s\n", progVersion)
		fmt.Printf("Built using %s(%s) for %s on %s\n", runtime.Version(), runtime.Compiler, runtime.GOOS, runtime.GOARCH)
		fmt.Print("License GPLv3+: GNU GPL version 3 or later <https://gnu.org/licenses/gpl.html>\n")
		fmt.Print("Direct Package Imports: runtime strings compress/gzip strconv io bufio slices encoding/json flag os/signal reflect fmt time syscall regexp os bytes crypto/sha256 sync path/filepath encoding/binary\n")
		return
	} else if versionRequested {
		fmt.Println(progVersion)
		return
	}

	// Act on User Choices
	if daemonMode {
		logReaderContinuous(logFileInput, outputFile)
	} else if searchMode {
		search(logFileInput, searchOpts)
	} else {
		printMessage(verbosityStandard, "No arguments specified or incorrect argument combination. Use '-h' or '--help' to guide your way.\n")
	}
}
