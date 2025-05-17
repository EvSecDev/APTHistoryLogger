#!/bin/bash
command -v git >/dev/null

function check_for_dev_artifacts() {
	local src repoDir headCommitHash lastReleaseCommitHash lastReleaseVersionNumber currentVersionNumber
	src=$1
	repoDir=$2

	echo "[*] Checking for development artifacts in source code..."

	# Quick check for any left over debug prints
	if grep -ER "DEBUG" "$src"/*.go
	then
		echo -e "   ${YELLOW}[?] WARNING${RESET}: Debug print found in source code. You might want to remove that before release."
	fi

	# Quick staticcheck check - ignoring punctuation in error strings
	cd "$src"
	set +e
	staticcheck ./*.go | grep -Ev "error strings should not"
	set -e
	cd "$repoDir"/

	echo -e "   ${GREEN}[+] DONE${RESET}"
}
