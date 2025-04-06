#!/bin/bash
if [ -z "$BASH_VERSION" ]
then
	echo "This script must be run in BASH."
	exit 1
fi

# Bail on any failure
set -e

# Check for required commands
command -v go >/dev/null
command -v sha256sum >/dev/null

# Vars
repoRoot=$(pwd)
SRCdir="src"
outputEXE="apthl"
debPkgName="apt-history-logger"

function usage {
	echo "Usage $0

Options:
  -b          Build the program using defaults
  -d          Build debian package
  -a <arch>   Architecture of compiled binary (amd64, arm64) [default: amd64]
  -o <os>     Which operating system to build for (linux, windows) [default: linux]
  -u          Update go packages for program
"
}

function check_for_dev_artifacts {
	# function args
	local srcDir=$1

        # Quick check for any left over debug prints
        if egrep -R "DEBUG" $srcDir/*.go
        then
                echo "  [-] Debug print found in source code. You might want to remove that before release."
        fi

	# Quick staticcheck check - ignoring punctuation in error strings
	cd $SRCdir
	set +e
	staticcheck *.go | egrep -v "error strings should not"
	set -e
	cd $repoRoot/
}

function fix_program_package_list_print {
    searchDir="$repoRoot/$SRCdir"
    mainFile=$(grep -il "func main() {" $searchDir/*.go | egrep -v "testing")

	# Hold cumulative (duplicated) imports from all go source files
	allImports=""

	# Loop all go source files
	for gosrcfile in $(find "$searchDir/" -maxdepth 1 -iname *.go)
	do
	        # Get space delimited single line list of imported package names (no quotes) for this go file
	        allImports+=$(cat $gosrcfile | awk '/import \(/,/\)/' | egrep -v "import \(|\)|^\n$" | sed -e 's/"//g' -e 's/\s//g' | tr '\n' ' ' | sed 's/  / /g')
	done

	# Put space delimited list of all the imports into an array
	IFS=' ' read -r -a pkgarr <<< "$allImports"

	# Create associative array for deduping
	declare -A packages

	# Add each import package to the associative array to delete dups
	for pkg in "${pkgarr[@]}"
	do
	        packages["$pkg"]=1
	done

	# Convert back to regular array
	allPackages=("${!packages[@]}")

	# search line in each program that contains package import list for version print
	packagePrintLine='fmt.Print("Direct Package Imports: '

	# Format package list into go print line
	newPackagePrintLine=$'\t\tfmt.Print("Direct Package Imports: '"${allPackages[@]}"'\\n")'

	# Remove testing package
	newPackagePrintLine=$(echo "$newPackagePrintLine" | sed 's/ testing//')

	# Write new package line into go source file that has main function
	sed -i "/$packagePrintLine/c\\$newPackagePrintLine" $mainFile
}

function binary {
	# Always ensure we start in the root of the repository
	cd $repoRoot/

	# Check for things not supposed to be in a release
	check_for_dev_artifacts "$SRCdir"

	# Check for new packages that were imported but not included in version output
	fix_program_package_list_print

	# Move into dir
	cd $SRCdir

	# Run tests
	go test

	# Vars for build
	inputGoSource="*.go"
	export CGO_ENABLED=0
	export GOARCH=$1
	export GOOS=$2

	# Build binary
	go build -o $repoRoot/$outputEXE -a -ldflags '-s -w -buildid= -extldflags "-static"' $inputGoSource
	cd $repoRoot
}

function debPkg {
	local arch=$1
	local os=$2

        # Always ensure we start in the root of the repository
        cd $repoRoot/

	# Build the binary
	binary "$arch" "$os"

	# Update control file with current binary version
	local binaryVersion=$($repoRoot/${outputEXE} --versionid | sed 's/v//')
	if [[ -z $binaryVersion ]]
	then
		echo "Unable to determine binary version" >&2
		exit 1
	fi
	sed -i 's/Version:.*/Version: '"$binaryVersion"'/' $repoRoot/packaging/DEBIAN/control

	# Always ensure we start in the root of the repository
	cd $repoRoot/

	# Temp dir for package
	mkdir $repoRoot/temp

	# Prepare directories and move files in
	local tempDir="$repoRoot/temp"
	local pkgDir="$tempDir/$debPkgName"
	mkdir -p $pkgDir
	mkdir -p $pkgDir/usr/bin
	mkdir -p $pkgDir/lib/systemd/system

	mv $outputEXE $pkgDir/usr/bin/
	cp $repoRoot/packaging/apthl.service $pkgDir/lib/systemd/system/
	cp -r $repoRoot/packaging/DEBIAN $pkgDir/
	cp $repoRoot/LICENSE.md $pkgDir/DEBIAN/copyright
	sed -i 's/Architecture: amd64/Architecture: '"$arch"'/' $pkgDir/DEBIAN/control

	chmod 755 $pkgDir/DEBIAN
	chmod 644 $pkgDir/DEBIAN/*
	chmod 755 $pkgDir/DEBIAN/{postrm,postinst,preinst}
	chmod 644 $pkgDir/lib/systemd/system/*
	chmod 755 $pkgDir/usr/bin/*

	# Move into build dir
	cd $tempDir

	# Create package
	dpkg-deb --verbose --root-owner-group --build apt-history-logger

	# Move package back to root
	mv ${pkgDir}.deb $repoRoot/
        cd $repoRoot/

	# Cleanup build dir
	rm -r $tempDir 2>/dev/null
}

function update_go_packages {
	# Always ensure we start in the root of the repository
	cd $repoRoot/

	# Move into src dir
	cd $SRCdir

	# Run go updates
	echo "==== Updating Go packages ===="
	go get -u all
	go mod verify
	go mod tidy
	echo "==== Updates Finished ===="
}

# START
# DEFAULT CHOICES
architecture="amd64"
os="linux"

# Argument parsing
while getopts 'a:o:bnudh' opt
do
	case "$opt" in
	  'a')
	    architecture="$OPTARG"
	    ;;
	  'b')
	    buildmode='true'
	    ;;
	  'o')
	    os="$OPTARG"
	    ;;
	  'u')
	    updatepackages='true'
	    ;;
          'd')
            buildDebPackage='true'
            ;;
	  'h')
	    usage
	    exit 0
 	    ;;
	  *)
	    usage
	    exit 0
 	    ;;
	esac
done

# Act on program args
if [[ $updatepackages == true ]]
then
	# Using the builtopt cd into the src dir and update packages then exit
	update_go_packages
	exit 0
elif [[ $buildmode == true ]]
then
	binary "$architecture" "$os"
	echo "Complete: binary built"
elif [[ $buildDebPackage == true ]]
then
	debPkg "$architecture" "$os"
else
	echo "unknown, bye"
	exit 1
fi

exit 0
