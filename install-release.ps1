[CmdletBinding()]
param(
    [string]$Version,
    [string]$BinDir,
    [string]$Repository,
    [string]$GitHubUrl,
    [string]$GitHubApiUrl
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
Set-StrictMode -Version 2.0

function Get-Setting {
    param([string]$Value, [string]$EnvironmentName, [string]$DefaultValue)
    if (-not [string]::IsNullOrWhiteSpace($Value)) { return $Value }
    $environmentValue = [Environment]::GetEnvironmentVariable($EnvironmentName)
    if (-not [string]::IsNullOrWhiteSpace($environmentValue)) { return $environmentValue }
    return $DefaultValue
}

function Fail { param([string]$Message) throw "clai installer: $Message" }

$Version = Get-Setting $Version "CLAI_VERSION" "latest"
$Repository = Get-Setting $Repository "CLAI_REPOSITORY" "merefield/clai"
$GitHubUrl = (Get-Setting $GitHubUrl "CLAI_GITHUB_URL" "https://github.com").TrimEnd("/")
$GitHubApiUrl = (Get-Setting $GitHubApiUrl "CLAI_GITHUB_API_URL" "https://api.github.com").TrimEnd("/")
$semanticTagPattern = '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-((?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$'

if ([string]::IsNullOrWhiteSpace($BinDir)) { $BinDir = [Environment]::GetEnvironmentVariable("CLAI_BIN_DIR") }
if ([string]::IsNullOrWhiteSpace($BinDir)) {
    if (-not [string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) { $BinDir = Join-Path $env:LOCALAPPDATA "Programs\clai\bin" }
    else { $BinDir = Join-Path $HOME ".local\bin" }
}

if ($Repository -notmatch '^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$') { Fail "CLAI_REPOSITORY must have the form owner/repository" }
if ((Test-Path -LiteralPath $BinDir) -and -not (Test-Path -LiteralPath $BinDir -PathType Container)) { Fail "CLAI_BIN_DIR exists and is not a directory: $BinDir" }
if ($Version -ne "latest" -and $Version -cnotmatch $semanticTagPattern) { Fail "invalid semantic release tag: $Version" }

switch ([Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()) {
    "X64" { $releaseArch = "amd64" }
    "Arm64" { $releaseArch = "arm64" }
    default { Fail "unsupported architecture" }
}

[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
$headers = @{ "User-Agent" = "clai-release-installer" }
$webRequestArgs = @{}
if ($PSVersionTable.PSVersion.Major -lt 6) { $webRequestArgs.UseBasicParsing = $true }
$temporaryDirectory = Join-Path ([IO.Path]::GetTempPath()) ("clai-release-install-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $temporaryDirectory | Out-Null

try {
    $releaseTag = $Version
    if ($releaseTag -eq "latest") {
        Write-Host "Resolving the latest CLAI release..."
        try { $release = Invoke-RestMethod -Uri "$GitHubApiUrl/repos/$Repository/releases/latest" -Headers $headers }
        catch { Fail "could not resolve the latest release: $($_.Exception.Message)" }
        if ($release -is [string]) { $release = $release | ConvertFrom-Json }
        $releaseTag = [string]$release.tag_name
    }
    if ($releaseTag -cnotmatch $semanticTagPattern) { Fail "invalid semantic release tag: $releaseTag" }
    $releaseVersion = $releaseTag -replace '^v', ''
    $archiveName = "clai_${releaseVersion}_windows_${releaseArch}.zip"
    $releaseUrl = "$GitHubUrl/$Repository/releases/download/$releaseTag"
    $archivePath = Join-Path $temporaryDirectory $archiveName
    $checksumsPath = Join-Path $temporaryDirectory "checksums.txt"

    Write-Host "Downloading CLAI $releaseTag for windows/$releaseArch..."
    try {
        Invoke-WebRequest -Uri "$releaseUrl/$archiveName" -Headers $headers -OutFile $archivePath @webRequestArgs
        Invoke-WebRequest -Uri "$releaseUrl/checksums.txt" -Headers $headers -OutFile $checksumsPath @webRequestArgs
    } catch { Fail "release download failed: $($_.Exception.Message)" }

    $matchingChecksums = @(Get-Content -LiteralPath $checksumsPath | ForEach-Object {
        if ($_ -match '^([0-9A-Fa-f]{64})\s+\*?(.+)$' -and $Matches[2] -eq $archiveName) { $Matches[1] }
    })
    if ($matchingChecksums.Count -ne 1) { Fail "checksums.txt does not contain exactly one valid checksum for $archiveName" }
    if ((Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant() -ne $matchingChecksums[0].ToLowerInvariant()) {
        Fail "SHA-256 checksum verification failed for $archiveName"
    }
    Write-Host "Verified the release checksum."

    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $archive = [IO.Compression.ZipFile]::OpenRead($archivePath)
    try {
        $entries = @($archive.Entries | Where-Object { $_.FullName -eq "clai.exe" })
        if ($entries.Count -ne 1) { Fail "release archive does not contain exactly one root-level clai.exe binary" }
        $candidate = Join-Path $temporaryDirectory "clai.exe"
        $source = $entries[0].Open()
        try {
            $destination = [IO.File]::Create($candidate)
            try { $source.CopyTo($destination) } finally { $destination.Dispose() }
        } finally { $source.Dispose() }
    } finally { $archive.Dispose() }

    $versionOutput = (& $candidate --version 2>&1 | Out-String).Trim()
    if ($LASTEXITCODE -ne 0) { Fail "the downloaded clai binary failed its version check" }
    if ($versionOutput -ne "clai version $releaseVersion") { Fail "the downloaded binary reported an unexpected version: $versionOutput" }

    New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
    $target = Join-Path $BinDir "clai.exe"
    if (Test-Path -LiteralPath $target -PathType Container) { Fail "installation target exists and is a directory: $target" }
    $stagedTarget = Join-Path $BinDir (".clai-" + [Guid]::NewGuid().ToString("N") + ".exe")
    try {
        Copy-Item -LiteralPath $candidate -Destination $stagedTarget
        Move-Item -LiteralPath $stagedTarget -Destination $target -Force
    } catch {
        if (Test-Path -LiteralPath $stagedTarget -PathType Leaf) { Remove-Item -LiteralPath $stagedTarget -Force }
        Fail "could not install into ${BinDir}: $($_.Exception.Message); set CLAI_BIN_DIR to a writable directory"
    }
    Write-Host "Installed clai to $target ($versionOutput)."
    if (@($env:PATH -split ';') -notcontains $BinDir) { Write-Host "Add $BinDir to PATH before invoking clai." }
} finally {
    if (Test-Path -LiteralPath $temporaryDirectory) { Remove-Item -LiteralPath $temporaryDirectory -Recurse -Force }
}
