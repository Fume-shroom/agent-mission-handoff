$ErrorActionPreference = "Stop"

$repo = "Fume-shroom/agent-mission-handoff"
$version = if ($env:AMH_VERSION) { $env:AMH_VERSION } else { "latest" }
$installDir = if ($env:AMH_INSTALL_DIR) { $env:AMH_INSTALL_DIR } else { Join-Path $HOME ".local\bin" }

$architecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
switch ($architecture) {
    "X64" { $arch = "x86_64" }
    "Arm64" { $arch = "arm64" }
    default { throw "amh installer: unsupported CPU architecture: $architecture" }
}

$asset = "amh_Windows_${arch}.zip"
if ($version -eq "latest") {
    $releaseBase = "https://github.com/$repo/releases/latest/download"
} else {
    $releaseBase = "https://github.com/$repo/releases/download/$version"
}

$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("amh-install-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $tempDir | Out-Null

try {
    $archivePath = Join-Path $tempDir $asset
    $checksumsPath = Join-Path $tempDir "checksums.txt"

    Write-Host "Downloading $asset..."
    Invoke-WebRequest -Uri "$releaseBase/$asset" -OutFile $archivePath
    Invoke-WebRequest -Uri "$releaseBase/checksums.txt" -OutFile $checksumsPath

    $checksumLine = Get-Content $checksumsPath | Where-Object { $_ -match "\s$([regex]::Escape($asset))$" } | Select-Object -First 1
    if (-not $checksumLine) { throw "amh installer: checksum for $asset was not published" }
    $expected = ($checksumLine -split "\s+")[0].ToLowerInvariant()
    $actual = (Get-FileHash -Path $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $expected) { throw "amh installer: checksum verification failed" }

    $packageDir = Join-Path $tempDir "package"
    Expand-Archive -Path $archivePath -DestinationPath $packageDir

    New-Item -ItemType Directory -Force -Path $installDir | Out-Null
    Copy-Item -Force (Join-Path $packageDir "amh.exe") (Join-Path $installDir "amh.exe")

    $skillSource = Join-Path $packageDir "skills\mission-handoff\SKILL.md"
    if (Test-Path $skillSource) {
        foreach ($agentHome in @((Join-Path $HOME ".codex"), (Join-Path $HOME ".claude"))) {
            $skillDir = Join-Path $agentHome "skills\mission-handoff"
            New-Item -ItemType Directory -Force -Path $skillDir | Out-Null
            Copy-Item -Force $skillSource (Join-Path $skillDir "SKILL.md")
        }
    }

    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $pathEntries = if ($userPath) { $userPath -split ";" } else { @() }
    if ($pathEntries -notcontains $installDir) {
        $newPath = if ($userPath) { "$userPath;$installDir" } else { $installDir }
        [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    }
    if (($env:Path -split ";") -notcontains $installDir) {
        $env:Path = "$env:Path;$installDir"
    }

    Write-Host ""
    Write-Host "Installed amh to $(Join-Path $installDir 'amh.exe')"
    Write-Host "Installed the Mission Handoff Skill for Codex and Claude Code."
    & (Join-Path $installDir "amh.exe") version
} finally {
    Remove-Item -Recurse -Force $tempDir -ErrorAction SilentlyContinue
}
