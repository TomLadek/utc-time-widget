<#
.SYNOPSIS
    Builds utc-time-widget.exe with Go running in Docker (no local Go needed).

.EXAMPLE
    .\build.ps1                 # build .\utc-time-widget.exe
.EXAMPLE
    .\build.ps1 -Test           # also cross-compile the tests and run them on Windows
.EXAMPLE
    .\build.ps1 -Release        # test, build and copy the exe to releases\v<VERSION>\
#>
param(
    [switch]$Test,
    [switch]$Release,
    [switch]$Force # with -Release: overwrite an existing release directory
)

$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot

$image = 'golang:1.26'
$goWinres = 'github.com/tc-hib/go-winres@v0.3.3'
$exe = 'utc-time-widget.exe'
$version = (Get-Content VERSION -Raw).Trim()
if ($version -notmatch '^\d+\.\d+\.\d+$') { throw "VERSION must be MAJOR.MINOR.PATCH, got '$version'" }

function Invoke-Go([string]$cmd) {
    # Go writes progress to stderr; don't let Windows PowerShell treat it as an error.
    $ErrorActionPreference = 'Continue'
    # Named volumes keep Go's module and build caches between the throw-away
    # containers. Docker creates them automatically on first use.
    docker run --rm `
        -v "${PSScriptRoot}:/src" -w /src `
        -v go-build-cache:/root/.cache/go-build `
        -v go-mod-cache:/go/pkg/mod `
        -e GOOS=windows -e GOARCH=amd64 -e CGO_ENABLED=0 `
        $image sh -c $cmd 2>&1 | ForEach-Object { "$_" }
    if ($LASTEXITCODE -ne 0) { throw "docker/go failed: $cmd" }
}

if ($Release) {
    $Test = $true
    $releaseDir = Join-Path 'releases' "v$version"
    if ((Test-Path $releaseDir) -and -not $Force) {
        throw "$releaseDir already exists. Bump VERSION or pass -Force."
    }
    if (-not (Select-String -Path CHANGELOG.md -Pattern "^## \[$([regex]::Escape($version))\]" -Quiet)) {
        throw "CHANGELOG.md has no '## [$version]' section."
    }
}

# Windows resources (version info, icon, manifest) go into a .syso file that
# 'go build' links in automatically.
$winres = @(
    # The tool itself runs inside the Linux container, so build it for the host.
    "env -u GOOS -u GOARCH go run $goWinres simply --arch amd64 --manifest gui --icon assets/app-icon.png"
    "--product-version $version --file-version $version"
    "--product-name 'UTC Time Widget' --file-description 'UTC Time Widget'"
    "--original-filename $exe --copyright 'Copyright (c) Tom Ladek. MIT License.'"
) -join ' '
$build = "go build -trimpath -ldflags='-s -w -H=windowsgui -X main.version=$version' -o $exe ."

Invoke-Go "go mod tidy && ! gofmt -l . | grep . && go vet ./... && $winres && $build"
Get-Item $exe | Select-Object Name, Length, LastWriteTime | Format-Table -AutoSize

if ($Test) {
    Invoke-Go "go test -c -o utc-time-widget.test.exe ."
    try {
        .\utc-time-widget.test.exe '-test.v' '-test.run=TestRenderLayout'
        if ($LASTEXITCODE -ne 0) { throw 'tests failed' }
    } finally {
        Remove-Item utc-time-widget.test.exe -ErrorAction SilentlyContinue
    }
}

if ($Release) {
    New-Item -ItemType Directory -Force $releaseDir | Out-Null
    Copy-Item $exe $releaseDir
    $hash = (Get-FileHash (Join-Path $releaseDir $exe) -Algorithm SHA256).Hash.ToLower()
    # LF line endings and no BOM, so 'sha256sum -c' works too.
    [IO.File]::WriteAllText((Join-Path $PSScriptRoot "$releaseDir\SHA256SUMS"), "$hash  $exe`n")
    "Release v$version written to $releaseDir"
}
