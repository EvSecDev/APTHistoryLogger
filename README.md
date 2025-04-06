# APT History Logger

## Overview

This is a simple daemon for Debian systems to read the history.log file produced by the Advanced Package Tool (APT) and produce a parsable single log line.

The APT `history.log` file is more difficult to parse than other traditional logs.
It contains a multi-line "event" of an APT operation (install, upgrade, ect.).
This program makes it easy to extract the information contained in those lines and format it into a machine-parsable output.

The output is written to stdout by default (which being a Systemd service means its all in Journald) or optionally to another log file.

The program also extracts additional information to include in the output log.
In total, the output JSON contains the following information:

- Start date
- End date
- Total elapsed seconds
- Total packages
- List of packages per operation type (Install, Upgrade, Remove, Purge)
- Package Name
- Package Architecture
- Package Version
- Package previous version, if applicable (Upgrades)

**Beware!** This program is still in active development.

Quick roadmap of features coming:

- Offer CLI-based searching of specific event types, package names, ect., from the local system
- Glob/Regex based searching
- Built-in remote syslog option
- Dedicated configuration file so specific CLI arguments are not needed

## Installation

A Debian package is provided for installation. Just `apt install ./apt-history-logger.deb` and you are done!
Binary is at `/usr/bin/apthl`, Systemd service is called `apthl.service`.

## Notes

### Log File Monitoring

This program utilizes Linux's `inotify` to efficiently monitor for new entries in the watched log file.
This efficiency offers low CPU utilization, resulting in ~8ms of total time used per hour when idling.
