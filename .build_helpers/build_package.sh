#!/bin/bash
function build_package() {
        local arch repoRoot debPkgName binaryVersion tempDir pkgDir
        arch=$1
        repoRoot=$2
    
        if [[ -z $outputEXE ]]
        then
            echo "Could not identify output binary name"
            return 1
        fi

        if [[ -z $fullNameProgramPrefix ]]
        then
            echo "Could not determine full program name from conf"
            return 1
        fi

        if ! [[ -x $(which dpkg-deb) ]]
        then
            echo "dpkg-deb command not found"
            return 1
        fi

        if [[ -z $outputEXE ]]
        then
            echo "Must provide file name of compiled binary"
            return 1
        fi

        # Always ensure we start in the root of the repository
        cd "$repoRoot/"

        # Update control file with current binary version
        binaryVersion=$("$repoRoot/$outputEXE" --versionid | sed 's/v//')
        if [[ -z $binaryVersion ]]
        then
                echo "Unable to determine binary version" >&2
                return 1
        fi
        sed -i 's/Version:.*/Version: '"$binaryVersion"'/' "$repoRoot/packaging/DEBIAN/control"

        debPkgName="apt-history-logger"
        debPkgOutFile="${outputEXE}-v${binaryVersion}-${arch}.deb"

        # Temp dir for package
        mkdir "$repoRoot/temp"
        trap 'rm -r "$tempDir" 2>/dev/null' ERR

        # Prepare directories and move files in
        tempDir="$repoRoot/temp"
        pkgDir="$tempDir/$debPkgName"
        mkdir -p "$pkgDir"
        mkdir -p "$pkgDir/usr/bin"
        mkdir -p "$pkgDir/lib/systemd/system"
        mkdir -p "$pkgDir/etc/apparmor.d"
        mkdir -p "$pkgDir/usr/share/bash-completion/completions"

        mv "$outputEXE" "$pkgDir/usr/bin/"
        cp "$repoRoot/packaging/apthl.service" "$pkgDir/lib/systemd/system/"
        cp "$repoRoot/packaging/usr.bin.apthl" "$pkgDir/etc/apparmor.d/"
        cp "$repoRoot/packaging/apthl_bash_completion" "$pkgDir/usr/share/bash-completion/completions/apthl"
        cp -r "$repoRoot/packaging/DEBIAN" "$pkgDir/"
        cp "$repoRoot/LICENSE.md" "$pkgDir/DEBIAN/copyright"
        sed -i 's/Architecture: amd64/Architecture: '"$arch"'/' "$pkgDir/DEBIAN/control"

        chmod 755 "$pkgDir/DEBIAN"
        chmod 644 "$pkgDir"/DEBIAN/*
        chmod 755 "$pkgDir"/DEBIAN/{postrm,postinst,preinst,prerm}
        chmod 644 "$pkgDir"/lib/systemd/system/*
        chmod 755 "$pkgDir"/usr/bin/*
        chmod 644 "$pkgDir"/etc/apparmor.d/*
        chmod 644 "$pkgDir/usr/share/bash-completion/completions/apthl"

        # Move into build dir
        cd "$tempDir"

        # Create package
        dpkg-deb --verbose --root-owner-group --build "$debPkgName"

        # Move package back to root
        mv "$pkgDir".deb "$repoRoot/"
        cd "$repoRoot/"
        mv "$debPkgName.deb" "$debPkgOutFile"
        sha256sum "$debPkgOutFile" > "$repoRoot/$debPkgOutFile.sha256"

        # Cleanup build dir
        rm -r "$tempDir" 2>/dev/null

        echo "Debian package built at $repoRoot/$debPkgOutFile"
}
