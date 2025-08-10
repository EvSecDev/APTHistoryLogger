# APT History Logger

## Overview

This is a simple daemon for Debian systems to read the history.log file produced by the Advanced Package Tool (APT) and produce a parsable single log line.

Additionally, this provides CLI-based searching of APT history log files for specific metadata such as package name, user, or time range.

The APT `history.log` file is more difficult to parse than other traditional logs.
It contains a multi-line "event" of an APT operation (install, upgrade, ect.).
This program makes it easy to extract the information contained in those lines and format it into a machine-parsable output.

The output is written to stdout by default (which being a Systemd service means its all in Journald) or optionally to another log file.

The program also extracts additional information to include in the output log.
In total, the output JSON contains the following information:

- Event ID
- Start date
- End date
- Total elapsed seconds
- Total packages
- List of packages per operation type (Install, Upgrade, Remove, Purge)
- APT operation true/false
- Package Name
- Package Architecture
- Package Version
- Package previous version, if applicable (Upgrades)

**Beware!** This program is still in active development.

## Installation

A Debian package is provided for installation.
Binary is at `/usr/bin/apthl`, Systemd service is called `apthl.service`.

## APTHL Help Menu

```bash
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
```

## Notes

### Log File Monitoring

This program utilizes Linux's `inotify` to efficiently monitor for new entries in the watched log file.
This efficiency offers low CPU utilization, resulting in ~8ms of total time used per hour when idling.
