[CmdletBinding()]
param(
    [switch]$BumpVersion,
    [string]$Output = 'ctrld-openapi-doc.exe'
)

$ErrorActionPreference = 'Stop'
$versionPath = Join-Path $PSScriptRoot 'VERSION'
$current = [System.IO.File]::ReadAllText($versionPath).Trim()
if ($current -notmatch '^v\d{4}\.[1-9]\d{0,2}\.[1-9]\d*$') {
    throw 'VERSION 必須符合 v四碼年.天序.手動升版次數，天序與次數從 1 開始。'
}
$parts = $current.Substring(1).Split('.')
$year = [int]$parts[0]
$day = [int]$parts[1]
$revision = [int]$parts[2]
if ($year -lt 1 -or $day -gt $(if ([datetime]::IsLeapYear($year)) { 366 } else { 365 })) {
    throw 'VERSION 中的年或天序無效。'
}

if ($BumpVersion) {
    $zone = [System.TimeZoneInfo]::FindSystemTimeZoneById('Taipei Standard Time')
    $today = [System.TimeZoneInfo]::ConvertTime([datetimeoffset]::UtcNow, $zone)
    $nextRevision = if ($year -eq $today.Year -and $day -eq $today.DayOfYear) { $revision + 1 } else { 1 }
    $current = "v$($today.Year).$($today.DayOfYear).$nextRevision"
    [System.IO.File]::WriteAllText($versionPath, "$current`r`n", [System.Text.UTF8Encoding]::new($false))
}

Push-Location $PSScriptRoot
try {
    & go build -trimpath -o $Output .
    if ($LASTEXITCODE -ne 0) {
        throw "Go 建置失敗，結束碼：$LASTEXITCODE"
    }
    Write-Output "已建置 $current → $Output"
}
finally {
    Pop-Location
}
